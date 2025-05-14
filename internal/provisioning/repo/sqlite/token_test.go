package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

func TestTokenDatabaseActions(t *testing.T) {
	tokenA := provisioning.Token{
		UUID:          uuid.MustParse(`8dae5ba3-2ad9-48a5-a7c4-188efb36fbb6`),
		UsesRemaining: 1,
		ExpireAt:      time.Now().Add(1 * time.Minute).UTC().Truncate(0), // Truncate to remove the monotonic clock.
		Description:   "token A",
	}

	tokenB := provisioning.Token{
		UUID:          uuid.MustParse(`e74417e0-e6d8-465a-b7bc-86d99a45ba49`),
		UsesRemaining: 10,
		ExpireAt:      time.Now().Add(10 * time.Minute).UTC().Truncate(0), // Truncate to remove the monotonic clock.
		Description:   "token B",
	}

	ctx := context.Background()

	// Create a new temporary database.
	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	token := sqlite.NewToken(tx)

	// Add token
	_, err = token.Create(ctx, tokenA)
	require.NoError(t, err)
	_, err = token.Create(ctx, tokenB)
	require.NoError(t, err)

	// Ensure we have two entries
	tokens, err := token.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 2)

	tokenIDs, err := token.GetAllUUIDs(ctx)
	require.NoError(t, err)
	require.Len(t, tokenIDs, 2)
	require.ElementsMatch(t, []uuid.UUID{
		uuid.MustParse("8dae5ba3-2ad9-48a5-a7c4-188efb36fbb6"),
		uuid.MustParse("e74417e0-e6d8-465a-b7bc-86d99a45ba49"),
	}, tokenIDs)

	// Should get back tokenA unchanged.
	dbTokenA, err := token.GetByUUID(ctx, tokenA.UUID)
	require.NoError(t, err)
	tokenA.ID = dbTokenA.ID
	require.Equal(t, tokenA, *dbTokenA)

	// Test updating a token.
	tokenB.UsesRemaining = 100
	err = token.Update(ctx, tokenB)
	require.NoError(t, err)
	dbTokenB, err := token.GetByUUID(ctx, tokenB.UUID)
	require.NoError(t, err)
	tokenB.ID = dbTokenB.ID
	require.Equal(t, tokenB, *dbTokenB)

	// Delete a token.
	err = token.DeleteByUUID(ctx, tokenA.UUID)
	require.NoError(t, err)
	_, err = token.GetByUUID(ctx, tokenA.UUID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have two tokens remaining.
	tokens, err = token.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	// Can't delete a token that doesn't exist.
	err = token.DeleteByUUID(ctx, uuid.MustParse(`66307d51-c379-4fb3-be5d-5c4c24ba7b21`))
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a token that doesn't exist.
	err = token.Update(ctx, tokenA)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
