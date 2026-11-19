//go:build !cgo

package transaction

import (
	"context"
	"errors"
)

// This is convenience for build components that don't require a database, like the CLI, to build without CGO.
var errNoCgo = errors.New("Database transactions require a build with cgo")

func Do(ctx context.Context, f func(ctx context.Context) error) error {
	return errNoCgo
}

func ForceTx(ctx context.Context, db DBTX, f func(context.Context, TX) error) error {
	return errNoCgo
}

func GetDBTX(ctx context.Context, db DBTX) DBTX {
	return db
}

func IsActive(ctx context.Context) bool {
	return false
}

func Begin(ctx context.Context) (context.Context, transaction) {
	return ctx, noCgoTransaction{}
}

type noCgoTransaction struct{}

func (noCgoTransaction) Commit() error {
	return errNoCgo
}

func (noCgoTransaction) Rollback() error {
	return nil
}
