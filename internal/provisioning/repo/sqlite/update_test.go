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
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateDatabaseActions(t *testing.T) {
	updateA := provisioning.Update{
		UUID:       uuid.MustParse(`e399698d-db42-53f6-97d7-1ad04dac34ba`),
		ExternalID: "lxc:incus-os:217816150",
		Components: api.UpdateComponents{
			api.UpdateComponentHypervisorOS,
		},
		Version:     "202505110348",
		PublishedAt: time.Date(2025, 5, 11, 4, 16, 36, 0, time.UTC),
		Severity:    "none",
		Files: provisioning.UpdateFiles{
			{
				Filename: "debug.raw.gz",
				URL:      "https://api.github.com/repos/lxc/incus-os/releases/assets/253608213",
				Size:     17884312,
			},
			{
				Filename: "incus.raw.gz",
				URL:      "https://api.github.com/repos/lxc/incus-os/releases/assets/253608198",
				Size:     219898968,
			},
		},
	}

	updateB := provisioning.Update{
		UUID:       uuid.MustParse(`d3a52570-df97-56bc-a849-0d634c945b8c`),
		ExternalID: "lxc:incus-os:217808146",
		Components: api.UpdateComponents{
			api.UpdateComponentHypervisorOS,
		},
		Version:     "202505110031",
		PublishedAt: time.Date(2025, 5, 11, 0, 56, 27, 0, time.UTC),
		Severity:    "none",
		Files: provisioning.UpdateFiles{
			{
				Filename: "debug.raw.gz",
				URL:      "https://api.github.com/repos/lxc/incus-os/releases/assets/253570621",
				Size:     17884331,
			},
			{
				Filename: "incus.raw.gz",
				URL:      "https://api.github.com/repos/lxc/incus-os/releases/assets/253570598",
				Size:     219903825,
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

	// Add update
	_, err = update.Create(ctx, updateA)
	require.NoError(t, err)
	_, err = update.Create(ctx, updateB)
	require.NoError(t, err)

	// Ensure we have two entries
	updates, err := update.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, updates, 2)

	updateIDs, err := update.GetAllUUIDs(ctx)
	require.NoError(t, err)
	require.Len(t, updateIDs, 2)
	require.ElementsMatch(t, []uuid.UUID{uuid.MustParse("e399698d-db42-53f6-97d7-1ad04dac34ba"), uuid.MustParse("d3a52570-df97-56bc-a849-0d634c945b8c")}, updateIDs)

	// Should get back updateA unchanged.
	dbUpdateA, err := update.GetByUUID(ctx, updateA.UUID)
	require.NoError(t, err)
	updateA.ID = dbUpdateA.ID
	require.Equal(t, updateA, *dbUpdateA)

	// Delete a update.
	err = update.DeleteByUUID(ctx, updateA.UUID)
	require.NoError(t, err)
	_, err = update.GetByUUID(ctx, updateA.UUID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have two updates remaining.
	updates, err = update.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	// Can't delete a update that doesn't exist.
	err = update.DeleteByUUID(ctx, uuid.Must(uuid.Parse(`66307d51-c379-4fb3-be5d-5c4c24ba7b21`)))
	require.ErrorIs(t, err, domain.ErrNotFound)
}
