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
	Commit() error
	Rollback() error
}

type transaction interface {
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

// ForceTx operates like Do, but instead of waiting for the first db call to open a transaction,
// one is opened immediately and passed to the function argument.
func ForceTx(ctx context.Context, db DBTX, f func(context.Context, TX) error) (err error) {
	tx, ok := db.(TX)
	if ok {
		// If the object is already a transaction, then just pass it through.
		return f(ctx, tx)
	}

	internalDB, ok := db.(dbtx)
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

	begunTx, err := internalDB.getDBTX(ctx)
	if err != nil {
		return err
	}

	tx, ok = begunTx.(TX)
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

	return nil
}

// GetDBTX returns the underlying TX if one exists in the context, otherwise returning the supplied DB.
func GetDBTX(ctx context.Context, db DBTX) DBTX {
	tc, ok := ctx.Value(tcKey{}).(*transactionContainer)
	if ok {
		// Transaction has started, return the underlying TX.
		if tc.tx != nil {
			return tc.tx
		}

		internalDBTX, ok := db.(dbtx)
		if !ok {
			return db
		}

		tx, err := internalDBTX.getDBTX(ctx)
		if err != nil {
			return db
		}

		return tx
	}

	// No transaction has started yet, return the DB.
	return db
}

// AfterCommit registers f to run after the transaction of ctx is committed.
// f does not run if the transaction is rolled back.
// Without a transaction in ctx, f runs immediately.
//
// f must not use the transaction. An error of f is returned by Commit, but the
// transaction stays committed.
func AfterCommit(ctx context.Context, f func() error) error {
	tc, ok := ctx.Value(tcKey{}).(*transactionContainer)
	if !ok {
		return f()
	}

	tc.lock.Lock()
	defer tc.lock.Unlock()

	tc.afterCommit = append(tc.afterCommit, f)

	return nil
}

func IsActive(ctx context.Context) bool {
	existingTC := ctx.Value(tcKey{})
	return existingTC != nil
}

func Begin(ctx context.Context) (context.Context, transaction) {
	if IsActive(ctx) {
		return ctx, &noopTransactionContainer{}
	}

	tc := &transactionContainer{}
	return context.WithValue(ctx, tcKey{}, tc), tc
}

type transactionContainer struct {
	tx          TX
	afterCommit []func() error
	lock        sync.Mutex
}

var _ transaction = &transactionContainer{}

func (t *transactionContainer) Commit() error {
	if t.tx != nil {
		err := t.tx.Commit()
		if err != nil {
			return err
		}
	}

	t.lock.Lock()
	afterCommit := t.afterCommit
	t.afterCommit = nil
	t.lock.Unlock()

	var errs []error

	for _, f := range afterCommit {
		err := f()
		if err != nil {
			errs = append(errs, fmt.Errorf("Transaction is committed, but a follow-up action failed: %w", err))
		}
	}

	return errors.Join(errs...)
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

var _ transaction = noopTransactionContainer{}

func (n noopTransactionContainer) Commit() error {
	return nil
}

func (n noopTransactionContainer) Rollback() error {
	return nil
}
