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
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateDatabaseActions(t *testing.T) {
	updateA := provisioning.Update{
		UUID:             uuid.MustParse(`e399698d-db42-53f6-97d7-1ad04dac34ba`),
		Version:          "202505110348",
		PublishedAt:      time.Date(2025, 5, 11, 4, 16, 36, 0, time.UTC),
		Severity:         images.UpdateSeverityNone,
		Origin:           "linuxcontainers.org",
		Channels:         []string{},
		UpstreamChannels: []string{"daily"},
		Status:           api.UpdateStatusReady,
		Changelog:        "Some changes",
		URL:              "/217816150",
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

	updateB := provisioning.Update{
		UUID:             uuid.MustParse(`d3a52570-df97-56bc-a849-0d634c945b8c`),
		Version:          "202505110031",
		PublishedAt:      time.Date(2025, 5, 11, 0, 56, 27, 0, time.UTC),
		Severity:         images.UpdateSeverityNone,
		Origin:           "alternative.org",
		Channels:         []string{},
		UpstreamChannels: []string{"stable", "daily"},
		Status:           api.UpdateStatusReady,
		Changelog:        "Other changes",
		URL:              "/217808146",
		Files: provisioning.UpdateFiles{
			{
				Filename:  "debug.raw.gz",
				Size:      17884331,
				Component: images.UpdateFileComponentDebug,
			},
			{
				Filename:  "incus.raw.gz",
				Size:      219903825,
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

	cannelSvc := provisioning.NewChannelService(sqlite.NewChannel(tx), nil)

	update := sqlite.NewUpdate(tx)

	// Add update
	err = update.Upsert(ctx, updateA)
	require.NoError(t, err)
	err = update.Upsert(ctx, updateB)
	require.NoError(t, err)

	// Ensure we have two entries
	updates, err := update.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, updates, 2)

	updateIDs, err := update.GetAllUUIDs(ctx)
	require.NoError(t, err)
	require.Len(t, updateIDs, 2)
	require.ElementsMatch(t, []uuid.UUID{uuid.MustParse("e399698d-db42-53f6-97d7-1ad04dac34ba"), uuid.MustParse("d3a52570-df97-56bc-a849-0d634c945b8c")}, updateIDs)

	// Ensure we have one entry with filter
	updates, err = update.GetAllWithFilter(ctx, provisioning.UpdateFilter{
		Origin: ptr.To("linuxcontainers.org"),
	})
	require.NoError(t, err)
	require.Len(t, updates, 1)

	updateIDs, err = update.GetAllUUIDsWithFilter(ctx, provisioning.UpdateFilter{
		Origin: ptr.To("alternative.org"),
	})
	require.NoError(t, err)
	require.Len(t, updateIDs, 1)
	require.ElementsMatch(t, []uuid.UUID{uuid.MustParse("d3a52570-df97-56bc-a849-0d634c945b8c")}, updateIDs)

	// Should get back updateA and updateB unchanged.
	dbUpdateA, err := update.GetByUUID(ctx, updateA.UUID)
	require.NoError(t, err)
	updateA.ID = dbUpdateA.ID
	updateA.LastUpdated = dbUpdateA.LastUpdated
	require.Equal(t, updateA, *dbUpdateA)

	// Upsert existing entry
	updateA.Severity = images.UpdateSeverityCritical
	err = update.Upsert(ctx, updateA)
	require.NoError(t, err)

	// Should get back updatedA changed.
	dbUpdateA, err = update.GetByUUID(ctx, updateA.UUID)
	updateA.LastUpdated = dbUpdateA.LastUpdated
	require.NoError(t, err)
	require.Equal(t, updateA, *dbUpdateA)

	// Delete a update.
	err = update.DeleteByUUID(ctx, updateA.UUID)
	require.NoError(t, err)
	_, err = update.GetByUUID(ctx, updateA.UUID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one updates remaining.
	updates, err = update.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	// Can't delete a update that doesn't exist.
	err = update.DeleteByUUID(ctx, uuid.MustParse(`66307d51-c379-4fb3-be5d-5c4c24ba7b21`))
	require.ErrorIs(t, err, domain.ErrNotFound)

	channel, err := cannelSvc.Create(ctx, provisioning.Channel{
		Name: "test-channel",
	})
	require.NoError(t, err)

	err = update.AssignChannels(ctx, updateB.UUID, []string{channel.Name})
	require.NoError(t, err)

	dbUpdateB, err := update.GetByUUID(ctx, updateB.UUID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"test-channel"}, dbUpdateB.Channels)

	updates, err = update.GetUpdatesByAssignedChannelName(ctx, "test-channel")
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.ElementsMatch(t, []string{"test-channel"}, updates[0].Channels)

	updates, err = update.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.ElementsMatch(t, []string{"test-channel"}, updates[0].Channels)

	updates, err = update.GetAllWithFilter(ctx, provisioning.UpdateFilter{
		Origin: ptr.To("alternative.org"),
	})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.ElementsMatch(t, []string{"test-channel"}, updates[0].Channels)
}
