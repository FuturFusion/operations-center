package dbschema

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

// copyRowsPerTx is the number of rows committed at once, so a transaction of the global database stays small.
const copyRowsPerTx = 10000

// copyMaxParams is the maximum number of parameters of a statement in SQLite.
const copyMaxParams = 32766

// copySkippedTables are filled by the schema of the destination itself.
var copySkippedTables = []string{"schema", "patches", "cluster_members"}

// CopyData copies the rows of all tables of src into the tables of the same name in dst, a fresh database with the same schema
// version. The rows dst got from its fresh schema are replaced by those of src. The rows are copied with multi-row inserts in
// transactions of limited size, since cowsql freezes or crashes on a single large statement and is slow with a commit per row.
func CopyData(ctx context.Context, src *sql.DB, dst *sql.DB) (err error) {
	tables, err := copiedTables(ctx, src)
	if err != nil {
		return fmt.Errorf("Failed to list tables: %w", err)
	}

	// The tables are copied in any order, the references are checked once all rows are in.
	conn, err := dst.Conn(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

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

	for _, table := range tables {
		_, err := conn.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", table))
		if err != nil {
			return fmt.Errorf("Failed to empty table %q: %w", table, err)
		}
	}

	for _, table := range tables {
		err := copyTable(ctx, src, conn, table)
		if err != nil {
			return fmt.Errorf("Failed to copy table %q: %w", table, err)
		}
	}

	var violations int
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&violations)
	if err != nil {
		return fmt.Errorf("Failed to check the foreign keys: %w", err)
	}

	if violations > 0 {
		return fmt.Errorf("Copied rows violate %d foreign key constraints", violations)
	}

	return nil
}

// copiedTables returns the tables of db, which are copied.
func copiedTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		if err != nil {
			return nil, err
		}

		if !slices.Contains(copySkippedTables, name) {
			tables = append(tables, name)
		}
	}

	return tables, rows.Err()
}

// copyTable copies the rows of table with multi-row inserts, committing every copyRowsPerTx rows.
func copyTable(ctx context.Context, src *sql.DB, dst *sql.Conn, table string) error {
	rows, err := src.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %q ORDER BY rowid", table))
	if err != nil {
		return err
	}

	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, fmt.Sprintf("%q", column))
	}

	rowsPerStmt := min(500, copyMaxParams/len(columns))
	placeholders := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ") + ")"
	insert := func(n int) string {
		return fmt.Sprintf("INSERT INTO %q (%s) VALUES %s", table, strings.Join(quoted, ", "), strings.TrimSuffix(strings.Repeat(placeholders+", ", n), ", "))
	}

	var tx *sql.Tx
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	args := make([]any, 0, rowsPerStmt*len(columns))
	inTx := 0

	flush := func(commit bool) error {
		if len(args) > 0 {
			if tx == nil {
				tx, err = dst.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
			}

			_, err := tx.ExecContext(ctx, insert(len(args)/len(columns)), args...)
			if err != nil {
				return err
			}

			inTx += len(args) / len(columns)
			args = args[:0]
		}

		if tx != nil && (commit || inTx >= copyRowsPerTx) {
			err := tx.Commit()
			tx = nil
			inTx = 0
			if err != nil {
				return err
			}
		}

		return nil
	}

	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		err := rows.Scan(pointers...)
		if err != nil {
			return err
		}

		args = append(args, values...)
		if len(args)/len(columns) == rowsPerStmt {
			err := flush(false)
			if err != nil {
				return err
			}
		}
	}

	err = rows.Err()
	if err != nil {
		return err
	}

	return flush(true)
}
