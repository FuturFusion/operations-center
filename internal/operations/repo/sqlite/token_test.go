package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/operations/repo/sqlite"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func TestTokenDatabaseActions(t *testing.T) {
	tokenA := operations.Token{
		UUID:          uuid.Must(uuid.Parse(`8dae5ba3-2ad9-48a5-a7c4-188efb36fbb6`)),
		UsesRemaining: 1,
		ExpireAt:      time.Now().Add(1 * time.Minute),
		Description:   "token A",
	}

	tokenB := operations.Token{
		UUID:          uuid.Must(uuid.Parse(`e74417e0-e6d8-465a-b7bc-86d99a45ba49`)),
		UsesRemaining: 10,
		ExpireAt:      time.Now().Add(10 * time.Minute),
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

	token := sqlite.NewToken(db)

	// Add token
	tokenA, err = token.Create(ctx, tokenA)
	require.NoError(t, err)
	tokenB, err = token.Create(ctx, tokenB)
	require.NoError(t, err)

	// Ensure we have three entries
	tokens, err := token.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 2)

	tokenIDs, err := token.GetAllIDs(ctx)
	require.NoError(t, err)
	require.Len(t, tokenIDs, 2)
	require.ElementsMatch(t, []string{"8dae5ba3-2ad9-48a5-a7c4-188efb36fbb6", "e74417e0-e6d8-465a-b7bc-86d99a45ba49"}, tokenIDs)

	// Should get back tokenA unchanged.
	dbTokenA, err := token.GetByID(ctx, tokenA.UUID)
	require.NoError(t, err)
	require.Equal(t, tokenA, dbTokenA)

	// Test updating a token.
	tokenB.UsesRemaining = 100
	dbTokenB, err := token.UpdateByID(ctx, tokenB)
	require.Equal(t, tokenB, dbTokenB)
	require.NoError(t, err)
	dbTokenB, err = token.GetByID(ctx, tokenB.UUID)
	require.NoError(t, err)
	require.Equal(t, tokenB, dbTokenB)

	// Delete a token.
	err = token.DeleteByID(ctx, tokenA.UUID)
	require.NoError(t, err)
	_, err = token.GetByID(ctx, tokenA.UUID)
	require.Error(t, err)

	// Should have two tokens remaining.
	tokens, err = token.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	// Can't delete a token that doesn't exist.
	err = token.DeleteByID(ctx, uuid.Must(uuid.Parse(`66307d51-c379-4fb3-be5d-5c4c24ba7b21`)))
	require.Error(t, err)

	// Can't update a token that doesn't exist.
	_, err = token.UpdateByID(ctx, tokenA)
	require.Error(t, err)
}
