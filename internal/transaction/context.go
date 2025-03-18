package transaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

type tcKey struct{}

type tx interface {
	DBTX
	Transaction
}

type Transaction interface {
	Commit() error
	Rollback() error
}

func Do(ctx context.Context, f func(ctx context.Context) error) (err error) {
	ctx, trans := Begin(ctx)
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			err = fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err)
			return
		}
	}()

	err = f(ctx)
	if err != nil {
		return err
	}

	err = trans.Commit()
	if err != nil {
		return fmt.Errorf("Failed commit transaction: %w", err)
	}

	return nil
}

func Begin(ctx context.Context) (context.Context, Transaction) {
	existingTC := ctx.Value(tcKey{})
	if existingTC != nil {
		return ctx, &noopTransactionContainer{}
	}

	tc := &transactionContainer{}
	return context.WithValue(ctx, tcKey{}, tc), tc
}

type transactionContainer struct {
	tx   tx
	lock sync.Mutex
}

var _ Transaction = &transactionContainer{}

func (t *transactionContainer) Commit() error {
	if t.tx == nil {
		return nil
	}

	return t.tx.Commit()
}

func (t *transactionContainer) Rollback() error {
	if t.tx == nil {
		return nil
	}

	err := t.tx.Rollback()
	if !errors.Is(err, sql.ErrTxDone) {
		return err
	}

	return nil
}

type noopTransactionContainer struct{}

var _ Transaction = noopTransactionContainer{}

func (n noopTransactionContainer) Commit() error {
	return nil
}

func (n noopTransactionContainer) Rollback() error {
	return nil
}
