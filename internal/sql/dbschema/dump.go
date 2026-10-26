package dbschema

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

// DumpTables returns a SQL text dump of all table's name, similar to
// sqlite3's dump feature.
func DumpTables(ctx context.Context, db dbdriver.DBTX) (string, error) {
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

// DumpSchema returns a SQL text dump of all rows across all tables, similar to
// sqlite3's dump feature.
func DumpSchema(ctx context.Context, db dbdriver.DBTX, schemaOnly bool) (string, error) {
	statements, err := dumpStatements(ctx, db, schemaOnly)
	if err != nil {
		return "", err
	}

	return "PRAGMA foreign_keys=OFF;\nBEGIN TRANSACTION;\n" + statements + "COMMIT;\n", nil
}

// dumpStatements returns the statements of the dump without a surrounding
// transaction, so they can be applied within an existing one.
func dumpStatements(ctx context.Context, db dbdriver.DBTX, schemaOnly bool) (string, error) {
	entitiesSchemas, entityNames, err := getEntitiesSchemas(ctx, db)
	if err != nil {
		return "", err
	}

	var builder strings.Builder

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

	err = rows.Err()
	if err != nil {
		return nil, nil, fmt.Errorf("Could not read table names and their schema: %w", err)
	}

	return tablesSchemas, names, nil
}

// getTableData gets all the data for a single table, returning a string slice where each element is an insert statement
// for the data.
func getTableData(ctx context.Context, db dbdriver.DBTX, table string) ([]string, error) {
	var statements []string

	// Query all rows.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s ORDER BY rowid", table))
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
				// sqlite stores booleans as integers.
				values[j] = "0"
				if v {
					values[j] = "1"
				}

			case string:
				// This is based on logic from dump_callback in sqlite source for sqlite3_db_dump function.
				v = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))

				if strings.Contains(v, "\r") {
					v = "replace(" + strings.ReplaceAll(v, "\r", "\\r") + ",'\\r',char(13))"
				}

				if strings.Contains(v, "\n") {
					v = "replace(" + strings.ReplaceAll(v, "\n", "\\n") + ",'\\n',char(10))"
				}

				values[j] = v

			case []byte:
				values[j] = fmt.Sprintf("'%s'", string(v))

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

		statement := fmt.Sprintf("INSERT INTO %s VALUES(%s);", table, strings.Join(values, ","))
		statements = append(statements, statement)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("Failed to read rows of table %q: %w", table, err)
	}

	return statements, nil
}
