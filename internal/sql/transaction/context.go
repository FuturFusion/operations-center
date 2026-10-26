package transaction

import (
	"context"
	"sync"
)

type TX interface {
	DBTX
	Commit() error
	Rollback() error
}

type transaction interface {
	Commit() error
	Rollback() error
}

// AfterCommit registers f to run after the transaction of ctx is committed.
// f does not run if the transaction is rolled back.
// Without a transaction in ctx, f runs immediately.
//
// f must not use the transaction. An error of f is returned by Commit, but the
// transaction stays committed.
func AfterCommit(ctx context.Context, f func() error) error {
	ac, ok := ctx.Value(afterCommitKey{}).(*afterCommit)
	if !ok {
		return f()
	}

	ac.lock.Lock()
	defer ac.lock.Unlock()

	ac.funcs = append(ac.funcs, f)

	return nil
}

type afterCommitKey struct{}

type afterCommit struct {
	funcs []func() error
	lock  sync.Mutex
}
