package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/sql/dump"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

// dumpTables returns a SQL text dump of all table's name, similar to
// sqlite3's dump feature.
func dumpTables(ctx context.Context, db dbdriver.DBTX) (string, error) {
	_, entityNames, err := getEntitiesSchemas(ctx, db)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for _, tableName := range entityNames {
		builder.WriteString(tableName + "\n")
	}

	return builder.String(), nil
}

// dumpSchema returns a SQL text dump of all rows across all tables, similar to
// sqlite3's dump feature.
func dumpSchema(ctx context.Context, db dbdriver.DBTX, schemaOnly bool) (string, error) {
	entitiesSchemas, entityNames, err := getEntitiesSchemas(ctx, db)
	if err != nil {
		return "", err
	}

	// Begin dump string.
	var builder strings.Builder
	builder.WriteString("PRAGMA foreign_keys=OFF;\n")
	builder.WriteString("BEGIN TRANSACTION;\n")

	// For each table, write the schema and optionally write the data.
	for _, tableName := range entityNames {
		builder.WriteString(entitiesSchemas[tableName][1] + "\n")

		if !schemaOnly && entitiesSchemas[tableName][0] == "table" {
			tableData, err := getTableData(ctx, db, tableName)
			if err != nil {
				return "", err
			}

			for _, stmt := range tableData {
				builder.WriteString(stmt + "\n")
			}
		}
	}

	// Sequences (unless the schemaOnly flag is true).
	if !schemaOnly {
		builder.WriteString("DELETE FROM sqlite_sequence;\n")

		tableData, err := getTableData(ctx, db, "sqlite_sequence")
		if err != nil {
			return "", fmt.Errorf("Failed to dump table sqlite_sequence: %w", err)
		}

		for _, stmt := range tableData {
			builder.WriteString(stmt + "\n")
		}
	}

	// Commit.
	builder.WriteString("COMMIT;\n")

	return builder.String(), nil
}

// getEntitiesSchemas gets all the tables, their kind, and their schema, as well as a list of entity names in their default order from
// the sqlite_master table. The returned map values are arrays of length 2 whose first element contains the entity type and the second
// contains it's schema.
func getEntitiesSchemas(ctx context.Context, db dbdriver.DBTX) (map[string][2]string, []string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, type, sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY rowid`)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not get table names and their schema: %w", err)
	}

	defer func() { _ = rows.Close() }()

	tablesSchemas := make(map[string][2]string)
	var names []string
	for rows.Next() {
		var name string
		var kind string
		var schema string
		err := rows.Scan(&name, &kind, &schema)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not scan table name and schema: %w", err)
		}

		// This is based on logic from dump_callback in sqlite source for sqlite3_db_dump function.
		if strings.HasPrefix(schema, `CREATE TABLE "`) {
			schema = strings.Replace(schema, "CREATE TABLE", "CREATE TABLE IF NOT EXISTS", 1)
		}

		names = append(names, name)
		tablesSchemas[name] = [2]string{kind, schema + ";"}
	}

	return tablesSchemas, names, nil
}

// getTableData gets all the data for a single table, returning a string slice where each element is an insert statement
// for the data.
func getTableData(ctx context.Context, db dbdriver.DBTX, table string) ([]string, error) {
	var statements []string

	// Query all rows.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s ORDER BY rowid", quoteIdentifier(table)))
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch rows for table %q: %w", table, err)
	}

	defer func() { _ = rows.Close() }()

	// Get the column names.
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("Failed to get columns for table %q: %w", table, err)
	}

	// Generate an INSERT statement for each row.
	for i := 0; rows.Next(); i++ {
		raw := make([]any, len(columns)) // Raw column values
		row := make([]any, len(columns))
		for i := range raw {
			row[i] = &raw[i]
		}

		err := rows.Scan(row...)
		if err != nil {
			return nil, fmt.Errorf("Failed to scan row %d in table %q: %w", i, table, err)
		}

		values := make([]string, len(columns))
		for j, v := range raw {
			switch v := v.(type) {
			case int64:
				values[j] = strconv.FormatInt(v, 10)

			case float64:
				values[j] = strconv.FormatFloat(v, 'g', -1, 64)

			case bool:
				// The driver returns BOOLEAN columns as bool. SQLite stores them as 0 or 1.
				values[j] = "0"
				if v {
					values[j] = "1"
				}

			case string:
				values[j] = quoteString(v)

			case []byte:
				// Write a BLOB literal like sqlite3 .dump. Raw bytes may contain quotes.
				values[j] = "X'" + strings.ToUpper(hex.EncodeToString(v)) + "'"

			case time.Time:
				// Try and match the sqlite3 .dump output format.
				format := "2006-01-02 15:04:05"

				if v.Nanosecond() > 0 {
					format = format + ".000000000"
				}

				format = format + "-07:00"

				values[j] = "'" + v.Format(format) + "'"

			default:
				if v != nil {
					return nil, fmt.Errorf("Bad type in column %q of row %d in table %q", columns[j], i, table)
				}

				values[j] = "NULL"
			}
		}

		statement := fmt.Sprintf("INSERT INTO %s VALUES(%s);", quoteIdentifier(table), strings.Join(values, ","))
		statements = append(statements, statement)
	}

	return statements, nil
}

func quoteString(s string) string {
	// Like sqlite3 .dump, write newlines and carriage returns as markers.
	// A marker must not occur in the text, or replace() changes the text on restore.
	literal := "'" + strings.ReplaceAll(s, "'", "''") + "'"

	if strings.Contains(s, "\n") {
		marker := unusedMarker(literal, `\n`, `\012`)
		literal = "replace(" + strings.ReplaceAll(literal, "\n", marker) + ",'" + marker + "',char(10))"
	}

	if strings.Contains(s, "\r") {
		marker := unusedMarker(literal, `\r`, `\015`)
		literal = "replace(" + strings.ReplaceAll(literal, "\r", marker) + ",'" + marker + "',char(13))"
	}

	return literal
}

func unusedMarker(s string, first string, second string) string {
	for _, marker := range []string{first, second} {
		if !strings.Contains(s, marker) {
			return marker
		}
	}

	for i := 0; ; i++ {
		marker := fmt.Sprintf("(%s%d)", first, i)
		if !strings.Contains(s, marker) {
			return marker
		}
	}
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (i *internalHandler) internalSQLSelect(ctx context.Context, query string, result *dump.SQLResult) error {
	result.Type = "select"

	rows, err := i.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("Failed to execute query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	result.Columns, err = rows.Columns()
	if err != nil {
		return fmt.Errorf("Failed to fetch column names: %w", err)
	}

	for rows.Next() {
		row := make([]any, len(result.Columns))
		rowPointers := make([]any, len(result.Columns))
		for i := range row {
			rowPointers[i] = &row[i]
		}

		err := rows.Scan(rowPointers...)
		if err != nil {
			return fmt.Errorf("Failed to scan row: %w", err)
		}

		for i, column := range row {
			// Convert bytes to string. This is safe as
			// long as we don't have any BLOB column type.
			data, ok := column.([]byte)
			if ok {
				row[i] = string(data)
			}
		}

		result.Rows = append(result.Rows, row)
	}

	err = rows.Err()
	if err != nil {
		return fmt.Errorf("Got a row error: %w", err)
	}

	return nil
}

func (i *internalHandler) internalSQLExec(ctx context.Context, query string, result *dump.SQLResult) error {
	result.Type = "exec"
	r, err := i.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("Failed to exec query: %w", err)
	}

	result.RowsAffected, err = r.RowsAffected()
	if err != nil {
		return fmt.Errorf("Failed to fetch affected rows: %w", err)
	}

	return nil
}
