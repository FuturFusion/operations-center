//go:build cgo

package transaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	clusterTx "github.com/cowsql/go-cowsql/cluster/db/transaction"
)

type Preparer = clusterTx.Preparer

var transactor clusterTx.Transactor

// Register is a convenience as we almost exclusively will be using the global database for all non-raft related actions.
func Register(t clusterTx.Transactor) {
	transactor = t
}

// Enable returns a DBTX with transactions enabled. Do and ForceTx will treat this as a local-only (non clustered) transactor.
func Enable(db clusterTx.DB) clusterTx.DBTX {
	return enabledDB{DBTX: clusterTx.Enable(db)}
}

// enabledDB wraps the DBTX returned by Enable.
type enabledDB struct {
	clusterTx.DBTX
}

func (e enabledDB) Prepare(query string) (*sql.Stmt, error) {
	preparer, ok := e.DBTX.(clusterTx.Preparer)
	if !ok {
		return nil, errors.New("Database does not support prepared statements")
	}

	return preparer.Prepare(query)
}

// localTransactor implements transaction.Transactor for a local-only DB.
type localTransactor struct {
	db clusterTx.DBTX
}

func (l localTransactor) DBTX() clusterTx.DBTX {
	return l.db
}

func (l localTransactor) MaxRetries() int {
	return 0
}

func (l localTransactor) OnTxStart(bool) func() {
	return nil
}

func (l localTransactor) TxTimeout() time.Duration {
	return 0
}

func (l localTransactor) EnterExclusive() error {
	return nil
}

// transactorFor returns one of:
// - the globally registered transactor.
// - a local-only transactor for the provided DBTX, if it is the returned DBTX from Enable.
// - an empty localTransactor.
func transactorFor(db DBTX) clusterTx.Transactor {
	enabled, ok := db.(enabledDB)
	if ok {
		return localTransactor{db: enabled.DBTX}
	}

	if transactor == nil {
		return localTransactor{}
	}

	return transactor
}

func Do(ctx context.Context, f func(ctx context.Context) error) (err error) {
	ctx, ac := withAfterCommit(ctx)

	// The transaction is opened lazily by GetDBTX on the database used.
	err = clusterTx.Do(ctx, transactorFor(nil), func(ctx context.Context) error {
		ac.reset()

		return f(ctx)
	})
	if err != nil {
		return err
	}

	return ac.run()
}

// ForceTx operates like Do, but instead of waiting for the first db call to open a transaction,
// one is opened immediately and passed to the function argument.
func ForceTx(ctx context.Context, db DBTX, f func(context.Context, TX) error) (err error) {
	ctx, ac := withAfterCommit(ctx)

	err = clusterTx.ForceTx(ctx, transactorFor(db), func(ctx context.Context, tx clusterTx.TX) error {
		ac.reset()

		return f(ctx, tx)
	})
	if err != nil {
		return err
	}

	return ac.run()
}

// GetDBTX returns the underlying TX if one exists in the context, otherwise returning the supplied DB.
func GetDBTX(ctx context.Context, db DBTX) DBTX {
	t := transactorFor(db)
	if t.DBTX() == nil {
		return db
	}

	// Outside of a transaction, a DBTX returned from Enable is kept, so ForceTx recognizes it.
	_, ok := db.(enabledDB)
	if ok && !IsActive(ctx) {
		return db
	}

	return clusterTx.GetDBTX(ctx, t.DBTX())
}

func IsActive(ctx context.Context) bool {
	return clusterTx.IsActive(ctx)
}

func Begin(ctx context.Context) (context.Context, transaction) {
	ctx, ac := withAfterCommit(ctx)
	ctx, tx := clusterTx.Begin(ctx)

	return ctx, &afterCommitTransaction{transaction: tx, runAfterCommit: ac.run}
}

// withAfterCommit prepares ctx to collect after commit functions, the returned afterCommit is nil if a transaction is active already.
func withAfterCommit(ctx context.Context) (context.Context, *afterCommit) {
	if IsActive(ctx) {
		return ctx, nil
	}

	ac := &afterCommit{}

	return context.WithValue(ctx, afterCommitKey{}, ac), ac
}

// reset drops the functions collected by a rolled back attempt of the transaction.
func (a *afterCommit) reset() {
	if a == nil {
		return
	}

	a.lock.Lock()
	a.funcs = nil
	a.lock.Unlock()
}

// run runs the collected functions.
func (a *afterCommit) run() error {
	if a == nil {
		return nil
	}

	a.lock.Lock()
	funcs := a.funcs
	a.funcs = nil
	a.lock.Unlock()

	var errs []error

	for _, f := range funcs {
		err := f()
		if err != nil {
			errs = append(errs, fmt.Errorf("Transaction is committed, but a follow-up action failed: %w", err))
		}
	}

	return errors.Join(errs...)
}

type afterCommitTransaction struct {
	transaction

	runAfterCommit func() error
}

func (t *afterCommitTransaction) Commit() error {
	err := t.transaction.Commit()
	if err != nil {
		return err
	}

	return t.runAfterCommit()
}
