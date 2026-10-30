package api

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/sql/dump"
)

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
