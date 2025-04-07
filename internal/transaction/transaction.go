package transaction

import (
	"context"
	"database/sql"
)

type db interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	DBTX
}

type dbtx struct {
	db db
}

var _ DBTX = dbtx{}

func Enable(db db) dbtx {
	return dbtx{
		db: db,
	}
}

func (t dbtx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db, err := t.getDBTX(ctx)
	if err != nil {
		return nil, err
	}

	return db.ExecContext(ctx, query, args...)
}

func (t dbtx) Prepare(query string) (*sql.Stmt, error) {
	return t.db.PrepareContext(context.Background(), query)
}

func (t dbtx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	db, err := t.getDBTX(ctx)
	if err != nil {
		return nil, err
	}

	return db.PrepareContext(ctx, query)
}

func (t dbtx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db, err := t.getDBTX(ctx)
	if err != nil {
		return nil, err
	}

	return db.QueryContext(ctx, query, args...)
}

func (t dbtx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db, err := t.getDBTX(ctx)
	if err != nil {
		// Workaround to create a *sql.Row with the private err field set to the
		// given error message.
		errDB, _ := sql.Open("sqlerrordriver", "")
		return errDB.QueryRow(err.Error())
	}

	return db.QueryRowContext(ctx, query, args...)
}

func (t dbtx) getDBTX(ctx context.Context) (DBTX, error) {
	tc, ok := ctx.Value(tcKey{}).(*transactionContainer)
	if !ok {
		// No transaction started, use regular DB connection.
		return t.db, nil
	}

	tc.lock.Lock()
	defer tc.lock.Unlock()

	if tc.tx == nil {
		// Transaction requested, but no DB transaction started yet.
		tx, err := t.db.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}

		tc.tx = tx
		return tx, nil
	}

	// Transaction found, return it.
	return tc.tx, nil
}
