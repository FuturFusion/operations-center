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
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
)

func TestTokenDatabaseActions(t *testing.T) {
	tokenA := provisioning.Token{
		UUID:          uuidgen.FromPattern(t, "1"),
		UsesRemaining: 1,
		ExpireAt:      time.Now().Add(1 * time.Minute).UTC().Truncate(0), // Truncate to remove the monotonic clock.
		Description:   "token A",
		Channel:       "stable",
	}

	tokenB := provisioning.Token{
		UUID:          uuidgen.FromPattern(t, "2"),
		UsesRemaining: 10,
		ExpireAt:      time.Now().Add(10 * time.Minute).UTC().Truncate(0), // Truncate to remove the monotonic clock.
		Description:   "token B",
		Channel:       "stable",
	}

	tokenBSeed1 := provisioning.TokenSeed{
		Token:       tokenB.UUID,
		Name:        "config 1",
		Description: "seed config 1",
		Public:      true,
		Seeds: provisioning.TokenImageSeedConfigs{
			Applications: map[string]any{
				"applications": true,
			},
			Network: map[string]any{
				"network": true,
			},
			Install: map[string]any{
				"install": true,
			},
		},
	}

	tokenBSeed2 := provisioning.TokenSeed{
		Token:       tokenB.UUID,
		Name:        "config 2",
		Description: "seed config B2",
		Public:      true,
		Seeds: provisioning.TokenImageSeedConfigs{
			Applications: map[string]any{
				"applications": true,
			},
			Network: map[string]any{
				"network": true,
			},
			Install: map[string]any{
				"install": true,
			},
		},
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
		uuidgen.FromPattern(t, "1"),
		uuidgen.FromPattern(t, "2"),
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

	// Should have one token remaining.
	tokens, err = token.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	// Can't delete a token that doesn't exist.
	err = token.DeleteByUUID(ctx, uuid.MustParse(`66307d51-c379-4fb3-be5d-5c4c24ba7b21`))
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a token that doesn't exist.
	err = token.Update(ctx, tokenA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Create TokenSeedConfig
	_, err = token.CreateTokenSeed(ctx, tokenBSeed1)
	require.NoError(t, err)
	_, err = token.CreateTokenSeed(ctx, tokenBSeed2)
	require.NoError(t, err)

	// Ensure we have two entries
	tokenSeeds, err := token.GetTokenSeedAll(ctx, tokenB.UUID)
	require.NoError(t, err)
	require.Len(t, tokenSeeds, 2)

	dbTokenSeedNames, err := token.GetTokenSeedAllNames(ctx, tokenB.UUID)
	require.NoError(t, err)
	require.Len(t, dbTokenSeedNames, 2)
	require.ElementsMatch(t, []string{
		"config 1",
		"config 2",
	}, dbTokenSeedNames)

	// Should get back tokenBSeedConfig unchanged.
	dbTokenBSeedConfig, err := token.GetTokenSeedByName(ctx, tokenB.UUID, "config 1")
	require.NoError(t, err)
	tokenBSeed1.ID = dbTokenBSeedConfig.ID
	tokenBSeed1.LastUpdated = dbTokenBSeedConfig.LastUpdated
	require.Equal(t, tokenBSeed1, *dbTokenBSeedConfig)

	// Test updating a token seed.
	tokenBSeed1.Description = "changed"
	err = token.UpdateTokenSeed(ctx, tokenBSeed1)
	require.NoError(t, err)
	dbTokenBSeed1, err := token.GetTokenSeedByName(ctx, tokenBSeed1.Token, tokenBSeed1.Name)
	require.NoError(t, err)
	tokenBSeed1.LastUpdated = dbTokenBSeed1.LastUpdated
	require.Equal(t, tokenB, *dbTokenB)

	// Delete a token seed.
	err = token.DeleteTokenSeedByName(ctx, tokenBSeed2.Token, tokenBSeed2.Name)
	require.NoError(t, err)
	_, err = token.GetTokenSeedByName(ctx, tokenBSeed2.Token, tokenBSeed2.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one token remaining.
	tokens, err = token.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	// Can't delete a token seed that doesn't exist.
	err = token.DeleteTokenSeedByName(ctx, tokenBSeed2.Token, tokenBSeed2.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a token seed that doesn't exist.
	err = token.UpdateTokenSeed(ctx, tokenBSeed2)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't create a token that already exists.
	_, err = token.CreateTokenSeed(ctx, tokenBSeed1)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
