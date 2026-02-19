package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

func TestUpdateChannelDatabaseActions(t *testing.T) {
	channelA := provisioning.Channel{
		Name:        "one",
		Description: "one description",
	}

	channelB := provisioning.Channel{
		Name:        "two",
		Description: "two description",
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

	// update := sqlite.NewUpdate(tx)
	channel := sqlite.NewChannel(tx)

	// Add channel
	_, err = channel.Create(ctx, channelA)
	require.NoError(t, err)
	_, err = channel.Create(ctx, channelB)
	require.NoError(t, err)

	// Ensure we have two entries
	channels, err := channel.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, channels, 2+1) // channels is always pre-populated with a default entry "stable"

	channelIDs, err := channel.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, channelIDs, 2+1)                                        // channels is always pre-populated with a default entry "stable"
	require.ElementsMatch(t, []string{"stable", "one", "two"}, channelIDs) // channels is always pre-populated with a default entry "stable"

	// Should get back channelA unchanged.
	dbChannelA, err := channel.GetByName(ctx, channelA.Name)
	require.NoError(t, err)
	channelA.ID = dbChannelA.ID
	channelA.LastUpdated = dbChannelA.LastUpdated
	require.Equal(t, channelA, *dbChannelA)

	dbChannelB, err := channel.GetByName(ctx, channelB.Name)
	require.NoError(t, err)
	channelB.ID = dbChannelB.ID
	channelB.LastUpdated = dbChannelB.LastUpdated
	require.Equal(t, channelB, *dbChannelB)

	// Test updating a channel.
	channelB.Description = "two description (updated)"
	err = channel.Update(ctx, channelB)
	require.NoError(t, err)
	dbChannelB, err = channel.GetByName(ctx, channelB.Name)
	require.NoError(t, err)
	channelB.ID = dbChannelB.ID
	channelB.LastUpdated = dbChannelB.LastUpdated
	require.Equal(t, channelB, *dbChannelB)

	// Delete a channel.
	err = channel.DeleteByName(ctx, channelA.Name)
	require.NoError(t, err)
	_, err = channel.GetByName(ctx, channelA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one channels remaining.
	channels, err = channel.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, channels, 1+1) // channels is always pre-populated with a default entry "stable"

	// Can't delete a channel that doesn't exist.
	err = channel.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a channel that doesn't exist.
	err = channel.Update(ctx, channelA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate channel.
	_, err = channel.Create(ctx, channelB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
