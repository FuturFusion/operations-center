package transaction

import (
	"context"
	"database/sql"
	"errors"
)

type tcKey struct{}

type tx interface {
	DBTX
	Commit() error
	Rollback() error
}

type transactionContainer struct {
	tx tx
}

type Transaction interface {
	Commit() error
	Rollback() error
}

func Begin(ctx context.Context) (context.Context, Transaction) {
	tc := &transactionContainer{}
	return context.WithValue(ctx, tcKey{}, tc), tc
}

func (t transactionContainer) Commit() error {
	if t.tx == nil {
		return nil
	}

	return t.tx.Commit()
}

func (t transactionContainer) Rollback() error {
	if t.tx == nil {
		return nil
	}

	err := t.tx.Rollback()
	if !errors.Is(err, sql.ErrTxDone) {
		return err
	}

	return nil
}
