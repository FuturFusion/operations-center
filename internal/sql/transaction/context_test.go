package transaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

func TestForceTx_inStartedTransacationRollback(t *testing.T) {
	// Setup DB.
	tmpDir := t.TempDir()

	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	setupDB(t, db)

	// DB Connection with transaction support.
	dbWithTransaction := transaction.Enable(db)
	dummySvc := dummy{
		db: dbWithTransaction,
	}

	ctx := context.Background()

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
	dummies, err = dummySvc.getAll(context.Background())
	require.NoError(t, err)
	require.Empty(t, dummies)
}

func TestForceTx_firstInTransacationRollback(t *testing.T) {
	// Setup DB.
	tmpDir := t.TempDir()

	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	setupDB(t, db)

	// DB Connection with transaction support.
	dbWithTransaction := transaction.Enable(db)
	dummySvc := dummy{
		db: dbWithTransaction,
	}

	ctx := context.Background()

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
	dummies, err = dummySvc.getAll(context.Background())
	require.NoError(t, err)
	require.Empty(t, dummies)
}

func TestForceTx_withoutTransacationRollback(t *testing.T) {
	// Setup DB.
	tmpDir := t.TempDir()

	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	setupDB(t, db)

	// DB Connection with transaction support.
	dbWithTransaction := transaction.Enable(db)
	dummySvc := dummy{
		db: dbWithTransaction,
	}

	ctx := context.Background()

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
	dummies, err = dummySvc.getAll(context.Background())
	require.NoError(t, err)
	require.Empty(t, dummies)
}
