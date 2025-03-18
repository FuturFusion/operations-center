package transaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

type tcKey struct{}

type TX interface {
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

	return err
}

// ForceTx operates like Do, but instead of waiting for the first db call to open a transaction,
// one is opened immediately and passed to the function argument.
func ForceTx(ctx context.Context, dbtx DBTX, f func(context.Context, TX) error) (err error) {
	db, ok := dbtx.(transaction)
	if !ok {
		return fmt.Errorf("Database does not support transactions")
	}

	// Check if there is an existing transaction in the context.
	ctx, trans := Begin(ctx)
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			err = fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err)
			return
		}
	}()

	// Begin should always re-use or create a transaction container.
	_, ok = ctx.Value(tcKey{}).(*transactionContainer)
	if !ok {
		return fmt.Errorf("Transaction context is invalid")
	}

	begunTx, err := db.getDBTX(ctx)
	if err != nil {
		return err
	}

	tx, ok := begunTx.(TX)
	if !ok {
		return fmt.Errorf("Transaction is invalid")
	}

	err = f(ctx, tx)
	if err != nil {
		return err
	}

	err = trans.Commit()
	if err != nil {
		return fmt.Errorf("Failed commit transaction: %w", err)
	}

	return err
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
	tx   TX
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
