package dbschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

// importStandalone imports local.db of a standalone Operations Center in varDir into dst, if dst has no schema table yet.
// cowsql crashes on a single Exec with many statements, so the rows are copied in batches of limited size. The schema table
// is written last. It marks the import as complete, so an interrupted import is cleaned up and repeated on the next start.
func importStandalone(ctx context.Context, dst *sql.DB, varDir string) (err error) {
	path := filepath.Join(varDir, "local.db")

	_, err = os.Stat(path) //nolint:forbidigo // SQLite accesses the database files by path as well.
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("Failed to check for existing local database: %w", err)
	}

	conn, err := dst.Conn(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	initialized, err := tableExists(ctx, conn, "schema")
	if err != nil {
		return fmt.Errorf("Failed to check if schema table exists: %w", err)
	}

	if initialized {
		return nil
	}

	src, err := dbdriver.Open(varDir)
	if err != nil {
		return err
	}

	defer func() { _ = src.Close() }()

	// The tables are created and filled in any order. Leftovers of an interrupted import are dropped without cascades.
	_, err = conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	if err != nil {
		return err
	}

	defer func() {
		_, fkErr := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")
		if err == nil {
			err = fkErr
		}
	}()

	entities, names, err := getEntitiesSchemas(ctx, src)
	if err != nil {
		return err
	}

	// The empty sqlite database of the tests has no schema table.
	_, ok := entities["schema"]
	if !ok {
		return nil
	}

	// Without a schema table in dst, its entities of the same name are left over from an interrupted import.
	err = dropImported(ctx, conn, names)
	if err != nil {
		return fmt.Errorf("Failed to clean up an interrupted import: %w", err)
	}

	// Like the dump of sqlite3: tables first, then the rows, then indexes, views and triggers.
	var tableStmts, otherStmts []string
	for _, name := range names {
		switch {
		case name == "schema":
			// The schema table is written last.
			continue

		case entities[name][0] == "table":
			tableStmts = append(tableStmts, entities[name][1])

		default:
			otherStmts = append(otherStmts, entities[name][1])
		}
	}

	err = execInTx(ctx, conn, tableStmts)
	if err != nil {
		return fmt.Errorf("Failed to create the imported tables: %w", err)
	}

	tables, err := copiedTables(ctx, src.DB, []string{"schema"})
	if err != nil {
		return fmt.Errorf("Failed to list the imported tables: %w", err)
	}

	for _, table := range tables {
		err := copyTable(ctx, src.DB, conn, table)
		if err != nil {
			return fmt.Errorf("Failed to import table %q: %w", table, err)
		}
	}

	err = copySequences(ctx, src.DB, conn, append(tables, "schema"))
	if err != nil {
		return fmt.Errorf("Failed to import the AUTOINCREMENT sequences: %w", err)
	}

	err = execInTx(ctx, conn, otherStmts)
	if err != nil {
		return fmt.Errorf("Failed to create the imported indexes, views and triggers: %w", err)
	}

	schemaRows, err := getTableData(ctx, src, "schema")
	if err != nil {
		return err
	}

	err = execInTx(ctx, conn, append([]string{entities["schema"][1]}, schemaRows...))
	if err != nil {
		return fmt.Errorf("Failed to import table %q: %w", "schema", err)
	}

	// Rename right after the import is complete. A global database that starts empty later must not import it again.
	err = os.Rename(path, path+".migrated") //nolint:forbidigo // SQLite accesses the database files by path as well.
	if err != nil {
		return fmt.Errorf("Failed to rename the imported standalone database: %w", err)
	}

	return nil
}

func tableExists(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
	var count int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// dropImported drops the views, triggers and tables of dst with one of the given names. Their indexes are dropped with them.
func dropImported(ctx context.Context, conn *sql.Conn, names []string) error {
	entities, err := droppableEntities(ctx, conn)
	if err != nil {
		return err
	}

	var stmts []string
	for _, entity := range entities {
		if slices.Contains(names, entity[1]) {
			stmts = append(stmts, fmt.Sprintf("DROP %s IF EXISTS %s", entity[0], quoteIdentifier(entity[1])))
		}
	}

	return execInTx(ctx, conn, stmts)
}

// droppableEntities returns the type and name of the views, triggers and tables in the order they can be dropped.
// It reads all rows before it returns, since the connection runs one statement at a time.
func droppableEntities(ctx context.Context, conn *sql.Conn) ([][2]string, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT type, name FROM sqlite_master
  WHERE type IN ('view', 'trigger', 'table') AND name NOT LIKE 'sqlite_%'
  ORDER BY CASE type WHEN 'view' THEN 0 WHEN 'trigger' THEN 1 ELSE 2 END`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var entities [][2]string
	for rows.Next() {
		var kind, name string
		err := rows.Scan(&kind, &name)
		if err != nil {
			return nil, err
		}

		entities = append(entities, [2]string{kind, name})
	}

	return entities, rows.Err()
}

// copySequences replaces the AUTOINCREMENT counters of the tables in dst with those of src.
// The copied rows only raise a counter to the highest id, not to the value of src.
func copySequences(ctx context.Context, src *sql.DB, dst *sql.Conn, tables []string) error {
	var count int
	err := src.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'`).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 || len(tables) == 0 {
		return nil
	}

	// The counters of the tables dst got before the import stay.
	args := make([]any, 0, len(tables))
	for _, table := range tables {
		args = append(args, table)
	}

	_, err = dst.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name IN ("+strings.TrimSuffix(strings.Repeat("?, ", len(tables)), ", ")+")", args...)
	if err != nil {
		return err
	}

	return copyTable(ctx, src, dst, "sqlite_sequence")
}

// execInTx runs each statement on its own, since cowsql crashes on a single Exec with many statements.
func execInTx(ctx context.Context, conn *sql.Conn, stmts []string) error {
	if len(stmts) == 0 {
		return nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	for _, stmt := range stmts {
		_, err := tx.ExecContext(ctx, stmt)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("Failed to execute %q: %w", stmt, err)
		}
	}

	return tx.Commit()
}
