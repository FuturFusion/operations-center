package transaction_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

func TestRollback(t *testing.T) {
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

	// Get dummies inside of transaction, 1 dummy expected.
	dummies, err = dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Len(t, dummies, 1)

	// Rollback transaction.
	err = trans.Rollback()
	require.NoError(t, err)

	// Query dummies with fresh context, expect to not get any dummies, since no
	// data has been persisted to the DB.
	dummies, err = dummySvc.getAll(context.Background())
	require.NoError(t, err)
	require.Empty(t, dummies)
}

func TestCommit(t *testing.T) {
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
	defer func() {
		err = trans.Rollback()
		require.NoError(t, err)
	}()

	// Add dummy in transaction.
	err = dummySvc.create(ctx)
	require.NoError(t, err)

	// Get dummies inside of transaction, 1 dummy expected.
	dummies, err = dummySvc.getAll(ctx)
	require.NoError(t, err)
	require.Len(t, dummies, 1)

	// Get dummy inside of transaction, name should match.
	id, err := dummySvc.getByID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, id)

	// Commit transaction.
	err = trans.Commit()
	require.NoError(t, err)

	// Query dummies with fresh context expect to get the dummy
	// committed in the previous transaction.
	dummies, err = dummySvc.getAll(context.Background())
	require.NoError(t, err)
	require.Len(t, dummies, 1)
}

func TestTransactionInTransaction(t *testing.T) {
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

	ctx, trans := transaction.Begin(ctx)
	defer func() {
		err = trans.Rollback()
		require.NoError(t, err)
	}()

	// Add dummy in transaction.
	err = dummySvc.create(ctx)
	require.NoError(t, err)

	ctx, innerTrans := transaction.Begin(ctx)
	defer func() {
		err = innerTrans.Rollback()
		require.NoError(t, err)
	}()

	// Add dummy in inner transaction.
	err = dummySvc.create(ctx)
	require.NoError(t, err)

	// Commit inner transaction.
	err = innerTrans.Commit()
	require.NoError(t, err)

	// Commit transaction.
	err = trans.Commit()
	require.NoError(t, err)
}

func TestDo_commit(t *testing.T) {
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

	err = transaction.Do(ctx, func(ctx context.Context) error {
		// Add dummy in transaction.
		return dummySvc.create(ctx)
	})
	require.NoError(t, err)
}

func TestDo_rollback(t *testing.T) {
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

	err = transaction.Do(ctx, func(ctx context.Context) error {
		// Add dummy in transaction.
		err := dummySvc.create(ctx)
		require.NoError(t, err)

		return errors.New("boom!")
	})
	require.Error(t, err)
}

func setupDB(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`
CREATE TABLE dummy (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL
);
`)
	require.NoError(t, err)
}

type dummy struct {
	db transaction.DBTX
}

func (d dummy) create(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `INSERT INTO dummy (id) values (null);`)
	return err
}

func (d dummy) getAll(ctx context.Context) ([]int, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT * FROM dummy`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	ids := make([]int, 0)
	for rows.Next() {
		var id int
		err = rows.Scan(&id)
		if err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return ids, nil
}

func (d dummy) getByID(ctx context.Context, id int) (int, error) {
	row := d.db.QueryRowContext(ctx, `SELECT * FROM dummy WHERE id = ?`, id)
	err := row.Scan(&id)
	return id, err
}
