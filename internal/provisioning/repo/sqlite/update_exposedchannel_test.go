package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateExposedchannelDatabaseActions(t *testing.T) {
	updateA := provisioning.Update{
		UUID:        uuid.MustParse(`e399698d-db42-53f6-97d7-1ad04dac34ba`),
		Version:     "202505110348",
		PublishedAt: time.Date(2025, 5, 11, 4, 16, 36, 0, time.UTC),
		Severity:    images.UpdateSeverityNone,
		Origin:      "linuxcontainers.org",
		Channels:    []string{"daily"},
		Status:      api.UpdateStatusReady,
		Changelog:   "Some changes",
		URL:         "/217816150",
		Files: provisioning.UpdateFiles{
			{
				Filename:  "debug.raw.gz",
				Size:      17884312,
				Component: images.UpdateFileComponentDebug,
			},
			{
				Filename:  "incus.raw.gz",
				Size:      219898968,
				Component: images.UpdateFileComponentIncus,
			},
		},
	}

	exposedchannelA := provisioning.Exposedchannel{
		Name:        "one",
		Description: "one description",
	}

	exposedchannelB := provisioning.Exposedchannel{
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

	update := sqlite.NewUpdate(tx)

	// Add exposedchannel
	_, err = update.CreateExposedchannel(ctx, exposedchannelA)
	require.NoError(t, err)
	_, err = update.CreateExposedchannel(ctx, exposedchannelB)
	require.NoError(t, err)

	// Ensure we have two entries
	exposedchannels, err := update.GetAllExposedchannels(ctx)
	require.NoError(t, err)
	require.Len(t, exposedchannels, 2+1) // exposedchannels is always pre-populated with a default entry "stable"

	exposedchannelIDs, err := update.GetAllExposedchannelNames(ctx)
	require.NoError(t, err)
	require.Len(t, exposedchannelIDs, 2+1)                                        // exposedchannels is always pre-populated with a default entry "stable"
	require.ElementsMatch(t, []string{"stable", "one", "two"}, exposedchannelIDs) // exposedchannels is always pre-populated with a default entry "stable"

	// Should get back exposedchannelA unchanged.
	dbExposedchannelA, err := update.GetExposedchannelByName(ctx, exposedchannelA.Name)
	require.NoError(t, err)
	exposedchannelA.ID = dbExposedchannelA.ID
	exposedchannelA.LastUpdated = dbExposedchannelA.LastUpdated
	require.Equal(t, exposedchannelA, *dbExposedchannelA)

	dbExposedchannelB, err := update.GetExposedchannelByName(ctx, exposedchannelB.Name)
	require.NoError(t, err)
	exposedchannelB.ID = dbExposedchannelB.ID
	exposedchannelB.LastUpdated = dbExposedchannelB.LastUpdated
	require.Equal(t, exposedchannelB, *dbExposedchannelB)

	// Test updating a exposedchannel.
	exposedchannelB.Description = "two description (updated)"
	err = update.UpdateExposedchannel(ctx, exposedchannelB)
	require.NoError(t, err)
	exposedchannelB.Name = "two-new"
	err = update.RenameExposedchannel(ctx, "two", exposedchannelB.Name)
	require.NoError(t, err)
	dbExposedchannelB, err = update.GetExposedchannelByName(ctx, exposedchannelB.Name)
	require.NoError(t, err)
	exposedchannelB.ID = dbExposedchannelB.ID
	exposedchannelB.LastUpdated = dbExposedchannelB.LastUpdated
	require.Equal(t, exposedchannelB, *dbExposedchannelB)

	// Delete a exposedchannel.
	err = update.DeleteExposedchannelByName(ctx, exposedchannelA.Name)
	require.NoError(t, err)
	_, err = update.GetExposedchannelByName(ctx, exposedchannelA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one exposedchannels remaining.
	exposedchannels, err = update.GetAllExposedchannels(ctx)
	require.NoError(t, err)
	require.Len(t, exposedchannels, 1+1) // exposedchannels is always pre-populated with a default entry "stable"

	// Can't delete a exposedchannel that doesn't exist.
	err = update.DeleteExposedchannelByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a exposedchannel that doesn't exist.
	err = update.UpdateExposedchannel(ctx, exposedchannelA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate exposedchannel.
	_, err = update.CreateExposedchannel(ctx, exposedchannelB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Add update
	err = update.Upsert(ctx, updateA)
	require.NoError(t, err)

	// Assign exposed channel to update
	err = update.AssignExposedchannel(ctx, updateA.UUID, exposedchannelB.Name)
	require.NoError(t, err)

	updates, err := update.GetUpdatesByAssignedExposedchannelName(ctx, exposedchannelB.Name)
	require.NoError(t, err)
	require.Len(t, updates, 1)
}

func TestUpdateExposedchannelAssociationTableDatabaseActions(t *testing.T) {
	updateA := provisioning.Update{
		UUID:        uuid.MustParse(`e399698d-db42-53f6-97d7-1ad04dac34ba`),
		Version:     "202505110348",
		PublishedAt: time.Date(2025, 5, 11, 4, 16, 36, 0, time.UTC),
		Severity:    images.UpdateSeverityNone,
		Origin:      "linuxcontainers.org",
		Channels:    []string{"daily"},
		Status:      api.UpdateStatusReady,
		Changelog:   "Some changes",
		URL:         "/217816150",
		Files: provisioning.UpdateFiles{
			{
				Filename:  "debug.raw.gz",
				Size:      17884312,
				Component: images.UpdateFileComponentDebug,
			},
			{
				Filename:  "incus.raw.gz",
				Size:      219898968,
				Component: images.UpdateFileComponentIncus,
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

	update := sqlite.NewUpdate(tx)

	err = update.Upsert(ctx, updateA)
	require.NoError(t, err)

	err = update.AssignExposedchannel(ctx, updateA.UUID, "stable")
}
