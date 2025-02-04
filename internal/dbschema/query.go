package dbschema

import (
	"context"
	"database/sql"
	"fmt"
)

// doesSchemaTableExist return whether the schema table is present in the
// database.
func doesSchemaTableExist(ctx context.Context, tx *sql.Tx) (bool, error) {
	statement := `
SELECT COUNT(name) FROM sqlite_master WHERE type = 'table' AND name = 'schema'
`
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return false, err
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return false, fmt.Errorf("schema table query returned no rows")
	}

	if rows.Err() != nil {
		return false, rows.Err()
	}

	if rows.Err() != nil {
		return false, rows.Err()
	}

	var count int

	err = rows.Scan(&count)
	if err != nil {
		return false, err
	}

	return count == 1, nil
}

// Return all versions in the schema table, in increasing order.
func selectSchemaVersions(ctx context.Context, tx *sql.Tx) ([]int, error) {
	query := `
SELECT version FROM schema ORDER BY version
`

	values := []int{}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var value int
		err := rows.Scan(&value)
		if err != nil {
			return nil, err
		}

		values = append(values, value)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return values, nil
}

// Create the schema table.
func createSchemaTable(tx *sql.Tx) error {
	statement := `
CREATE TABLE schema (
    id         INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    version    INTEGER NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE (version)
)
`
	_, err := tx.Exec(statement)
	return err
}

// Insert a new version into the schema table.
func insertSchemaVersion(tx *sql.Tx, newVersion int) error {
	statement := `
INSERT INTO schema (version, updated_at) VALUES (?, strftime("%s"))
`
	_, err := tx.Exec(statement, newVersion)
	return err
}
