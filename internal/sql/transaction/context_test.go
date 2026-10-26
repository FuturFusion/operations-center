package transaction_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	clusterTx "github.com/cowsql/go-cowsql/cluster/db/transaction"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

func TestForceTx_inStartedTransacationRollback(t *testing.T) {
	t.Parallel()

	// Setup DB.
	tmpDir := t.TempDir()

	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	setupDB(t, db.DB)

	// DB Connection with transaction support.
	dbWithTransaction := transaction.Enable(db)
	dummySvc := dummy{
		db: dbWithTransaction,
	}

	ctx := t.Context()

	// Get dummies from empty db, no dummies expected.
	dummies, err := dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Empty(t, dummies)

	// Start transaction.
	ctx, trans := transaction.Begin(ctx)

	// Add source in transaction.
	err = dummySvc.create(ctx)
	require.NoError(t, err)

	// Perform DB operation with ForceTx as second operation in started transaction.
	err = transaction.ForceTx(ctx, transaction.GetDBTX(ctx, dbWithTransaction), func(ctx context.Context, tx transaction.TX) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO dummy (id) values (null);`)
		return err
	})
	require.NoError(t, err)

	// Get dummies inside of transaction, 2 dummy expected.
	dummies, err = dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Len(t, dummies, 2)

	// Rollback transaction.
	err = trans.Rollback()
	require.NoError(t, err)

	// Query dummies with fresh context, expect to not get any dummies, since no
	// data has been persisted to the DB.
	dummies, err = dummySvc.getAll(t.Context())
	require.NoError(t, err)
	require.Empty(t, dummies)
}

func TestForceTx_firstInTransacationRollback(t *testing.T) {
	t.Parallel()

	// Setup DB.
	tmpDir := t.TempDir()

	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	setupDB(t, db.DB)

	// DB Connection with transaction support.
	dbWithTransaction := transaction.Enable(db)
	dummySvc := dummy{
		db: dbWithTransaction,
	}

	ctx := t.Context()

	// Get dummies from empty db, no dummies expected.
	dummies, err := dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Empty(t, dummies)

	// Start transaction.
	ctx, trans := transaction.Begin(ctx)

	// Perform DB operation with ForceTx as first operation in started transaction.
	err = transaction.ForceTx(ctx, transaction.GetDBTX(ctx, dbWithTransaction), func(ctx context.Context, tx transaction.TX) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO dummy (id) values (null);`)
		return err
	})
	require.NoError(t, err)

	// Add source in transaction.
	err = dummySvc.create(ctx)
	require.NoError(t, err)

	// Get dummies inside of transaction, 2 dummy expected.
	dummies, err = dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Len(t, dummies, 2)

	// Rollback transaction.
	err = trans.Rollback()
	require.NoError(t, err)

	// Query dummies with fresh context, expect to not get any dummies, since no
	// data has been persisted to the DB.
	dummies, err = dummySvc.getAll(t.Context())
	require.NoError(t, err)
	require.Empty(t, dummies)
}

func TestForceTx_withoutTransacationRollback(t *testing.T) {
	t.Parallel()

	// Setup DB.
	tmpDir := t.TempDir()

	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	setupDB(t, db.DB)

	// DB Connection with transaction support.
	dbWithTransaction := transaction.Enable(db)
	dummySvc := dummy{
		db: dbWithTransaction,
	}

	ctx := t.Context()

	// Get dummies from empty db, no dummies expected.
	dummies, err := dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Empty(t, dummies)

	// Perform DB operation with ForceTx as first operation in started transaction.
	err = transaction.ForceTx(ctx, transaction.GetDBTX(ctx, dbWithTransaction), func(ctx context.Context, tx transaction.TX) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO dummy (id) values (null);`)
		require.NoError(t, err)

		return errors.New("force rollback")
	})
	require.ErrorContains(t, err, "force rollback")

	// Query dummies with fresh context, expect to not get any dummies, since no
	// data has been persisted to the DB.
	dummies, err = dummySvc.getAll(t.Context())
	require.NoError(t, err)
	require.Empty(t, dummies)
}

func TestAfterCommit(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		run  func(t *testing.T, calls *[]string) error

		assertErr require.ErrorAssertionFunc
		wantCalls []string
	}{
		{
			name: "runs immediately without transaction",
			run: func(t *testing.T, calls *[]string) error {
				t.Helper()

				err := transaction.AfterCommit(t.Context(), func() error {
					*calls = append(*calls, "after commit")
					return nil
				})

				*calls = append(*calls, "done")

				return err
			},

			assertErr: require.NoError,
			wantCalls: []string{"after commit", "done"},
		},
		{
			name: "runs after the outermost transaction is committed",
			run: func(t *testing.T, calls *[]string) error {
				t.Helper()

				return transaction.Do(t.Context(), func(ctx context.Context) error {
					err := transaction.Do(ctx, func(ctx context.Context) error {
						return transaction.AfterCommit(ctx, func() error {
							*calls = append(*calls, "after commit")
							return nil
						})
					})

					*calls = append(*calls, "inner done")

					return err
				})
			},

			assertErr: require.NoError,
			wantCalls: []string{"inner done", "after commit"},
		},
		{
			name: "does not run if the transaction is rolled back",
			run: func(t *testing.T, calls *[]string) error {
				t.Helper()

				return transaction.Do(t.Context(), func(ctx context.Context) error {
					err := transaction.AfterCommit(ctx, func() error {
						*calls = append(*calls, "after commit")
						return nil
					})
					require.NoError(t, err)

					return errBoom
				})
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, errBoom, a...)
			},
			wantCalls: nil,
		},
		{
			name: "error is returned by the commit and all functions run",
			run: func(t *testing.T, calls *[]string) error {
				t.Helper()

				return transaction.Do(t.Context(), func(ctx context.Context) error {
					err := transaction.AfterCommit(ctx, func() error {
						*calls = append(*calls, "first")
						return errBoom
					})
					require.NoError(t, err)

					return transaction.AfterCommit(ctx, func() error {
						*calls = append(*calls, "second")
						return nil
					})
				})
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, errBoom, a...)
			},
			wantCalls: []string{"first", "second"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string

			err := tc.run(t, &calls)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}

// retryingTransactor retries transactions like the transactor of the cluster database.
type retryingTransactor struct {
	db clusterTx.DBTX
}

func (r retryingTransactor) DBTX() clusterTx.DBTX     { return r.db }
func (r retryingTransactor) MaxRetries() int          { return 3 }
func (r retryingTransactor) OnTxStart(bool) func()    { return nil }
func (r retryingTransactor) TxTimeout() time.Duration { return 0 }
func (r retryingTransactor) EnterExclusive() error    { return nil }

// Not parallel, since it registers the transactor of all transactions.
func TestAfterCommit_retry(t *testing.T) {
	db, err := sqlite.Open(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() {
		transaction.Register(nil)

		err = db.Close()
		require.NoError(t, err)
	})

	transaction.Register(retryingTransactor{db: clusterTx.Enable(db)})

	var calls []string
	attempt := 0

	err = transaction.Do(t.Context(), func(ctx context.Context) error {
		attempt++

		err := transaction.AfterCommit(ctx, func() error {
			calls = append(calls, "attempt "+strconv.Itoa(attempt))
			return nil
		})
		require.NoError(t, err)

		if attempt == 1 {
			return errors.New("database is locked")
		}

		return nil
	})
	require.NoError(t, err)

	require.Equal(t, 2, attempt)
	require.Equal(t, []string{"attempt 2"}, calls)
}

func TestIsActive(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	require.False(t, transaction.IsActive(ctx))

	err := transaction.Do(ctx, func(ctx context.Context) error {
		require.True(t, transaction.IsActive(ctx))

		return nil
	})
	require.NoError(t, err)
}
