package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// SelectStrings executes a statement which must yield rows with a single string
// column. It returns the list of column values.
func SelectStrings(ctx context.Context, tx transaction.DBTX, query string, args ...any) ([]string, error) {
	values := []string{}
	scan := func(rows *sql.Rows) error {
		var value string
		err := rows.Scan(&value)
		if err != nil {
			return err
		}

		values = append(values, value)
		return nil
	}

	err := scanSingleColumn(ctx, tx, query, args, scan)
	if err != nil {
		return nil, err
	}

	return values, nil
}

// Execute the given query and run the given scanner for every row yielded.
func scanSingleColumn(ctx context.Context, tx transaction.DBTX, query string, args []any, scan scanFunc) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}

	defer func() {
		err := rows.Close()
		if err != nil {
			slog.WarnContext(ctx, "Failed to close rows", logger.Err(err))
		}
	}()

	for rows.Next() {
		err := scan(rows)
		if err != nil {
			return err
		}
	}

	err = rows.Err()
	if err != nil {
		return err
	}

	return nil
}

// SelectIntegers executes a statement which must yield rows with a single integer
// column. It returns the list of column values.
func SelectIntegers(ctx context.Context, tx transaction.DBTX, query string, args ...any) ([]int, error) {
	values := []int{}
	scan := func(rows *sql.Rows) error {
		var value int
		err := rows.Scan(&value)
		if err != nil {
			return err
		}

		values = append(values, value)
		return nil
	}

	err := scanSingleColumn(ctx, tx, query, args, scan)
	if err != nil {
		return nil, err
	}

	return values, nil
}

// InsertStrings inserts a new row for each of the given strings, using the
// given insert statement template, which must define exactly one insertion
// column and one substitution placeholder for the values. For example:
// InsertStrings(tx, "INSERT INTO foo(name) VALUES %s", []string{"bar"}).
func InsertStrings(ctx context.Context, tx transaction.DBTX, stmt string, values []string) error {
	n := len(values)

	if n == 0 {
		return nil
	}

	params := make([]string, n)
	args := make([]any, n)
	for i, value := range values {
		params[i] = "(?)"
		args[i] = value
	}

	stmt = fmt.Sprintf(stmt, strings.Join(params, ", "))
	_, err := tx.ExecContext(ctx, stmt, args...)
	return err
}

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

// Function to scan a single row.
type scanFunc func(*sql.Rows) error
