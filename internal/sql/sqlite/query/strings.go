package query

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// Count returns the number of rows in the given table.
func Count(ctx context.Context, tx transaction.DBTX, table string, where string, args ...any) (int, error) {
	stmt := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if where != "" {
		stmt += fmt.Sprintf(" WHERE %s", where)
	}

	rows, err := tx.QueryContext(ctx, stmt, args...)
	if err != nil {
		return -1, err
	}

	defer func() {
		err := rows.Close()
		if err != nil {
			slog.WarnContext(ctx, "Failed to close rows", logger.Err(err))
		}
	}()

	// Ensure we read one and only one row.
	if !rows.Next() {
		return -1, errors.New("no rows returned")
	}

	var count int
	err = rows.Scan(&count)
	if err != nil {
		return -1, errors.New("failed to scan count column")
	}

	if rows.Next() {
		return -1, errors.New("more than one row returned")
	}

	err = rows.Err()
	if err != nil {
		return -1, err
	}

	return count, nil
}
