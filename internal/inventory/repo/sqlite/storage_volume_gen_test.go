// Code generated by generate-inventory; DO NOT EDIT.

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	incusapi "github.com/lxc/incus/v6/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/inventory"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/ptr"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestStorageVolumeDatabaseActions(t *testing.T) {
	testToken := provisioning.Token{
		UsesRemaining: 10,
		ExpireAt:      time.Now().Add(1 * time.Minute),
	}

	testClusterA := provisioning.Cluster{
		Name:          "one",
		ConnectionURL: "https://cluster-one/",
		ServerNames:   []string{"one"},
		LastUpdated:   time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	testClusterB := provisioning.Cluster{
		Name:          "two",
		ConnectionURL: "https://cluster-two/",
		ServerNames:   []string{"two"},
		LastUpdated:   time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	testServerA := provisioning.Server{
		Name:          "one",
		ConnectionURL: "https://server-one/",
		Certificate: `-----BEGIN CERTIFICATE-----
server-one
-----END CERTIFICATE-----
`,
		Type:        api.ServerTypeIncus,
		Status:      api.ServerStatusReady,
		LastUpdated: time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	testServerB := provisioning.Server{
		Name:          "two",
		ConnectionURL: "https://server-two/",
		Certificate: `-----BEGIN CERTIFICATE-----
server-two
-----END CERTIFICATE-----
`,
		Type:        api.ServerTypeIncus,
		Status:      api.ServerStatusReady,
		LastUpdated: time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	storageVolumeA := inventory.StorageVolume{
		Cluster:         "one",
		Server:          "one",
		ProjectName:     "one",
		StoragePoolName: "parent one",
		Name:            "one",
		Type:            "custom",
		Object:          incusapi.StorageVolume{},
		LastUpdated:     time.Now(),
	}

	storageVolumeA.DeriveUUID()

	storageVolumeB := inventory.StorageVolume{
		Cluster:         "two",
		Server:          "two",
		ProjectName:     "two",
		StoragePoolName: "parent one",
		Name:            "two",
		Type:            "custom",
		Object:          incusapi.StorageVolume{},
		LastUpdated:     time.Now(),
	}

	storageVolumeB.DeriveUUID()

	client := &adapterMock.ServerClientPortMock{
		PingFunc: func(ctx context.Context, server provisioning.Server) error {
			return nil
		},
		GetResourcesFunc: func(ctx context.Context, server provisioning.Server) (api.HardwareData, error) {
			return api.HardwareData{}, nil
		},
		GetOSDataFunc: func(ctx context.Context, server provisioning.Server) (api.OSData, error) {
			return api.OSData{}, nil
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

	tokenSvc := provisioning.NewTokenService(provisioningSqlite.NewToken(tx))
	serverSvc := provisioning.NewServerService(provisioningSqlite.NewServer(tx), client, tokenSvc)
	clusterSvc := provisioning.NewClusterService(provisioningSqlite.NewCluster(tx), serverSvc, nil)

	storageVolume := inventorySqlite.NewStorageVolume(tx)

	// Cannot add an storageVolume with an invalid server.
	_, err = storageVolume.Create(ctx, storageVolumeA)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Add token.
	testToken, err = tokenSvc.Create(ctx, testToken)
	require.NoError(t, err)

	// Add dummy servers.
	_, err = serverSvc.Create(ctx, testToken.UUID, testServerA)
	require.NoError(t, err)
	_, err = serverSvc.Create(ctx, testToken.UUID, testServerB)
	require.NoError(t, err)

	// Add dummy clusters.
	_, err = clusterSvc.Create(ctx, testClusterA)
	require.NoError(t, err)
	_, err = clusterSvc.Create(ctx, testClusterB)
	require.NoError(t, err)

	// Add storage_volumes
	storageVolumeA, err = storageVolume.Create(ctx, storageVolumeA)
	require.NoError(t, err)
	require.Equal(t, "one", storageVolumeA.Cluster)

	storageVolumeB, err = storageVolume.Create(ctx, storageVolumeB)
	require.NoError(t, err)
	require.Equal(t, "two", storageVolumeB.Cluster)

	// Ensure we have two entries without filter
	storageVolumeUUIDs, err := storageVolume.GetAllUUIDsWithFilter(ctx, inventory.StorageVolumeFilter{})
	require.NoError(t, err)
	require.Len(t, storageVolumeUUIDs, 2)
	require.ElementsMatch(t, []uuid.UUID{storageVolumeA.UUID, storageVolumeB.UUID}, storageVolumeUUIDs)

	// Ensure we have two entries without filter
	dbStorageVolume, err := storageVolume.GetAllWithFilter(ctx, inventory.StorageVolumeFilter{})
	require.NoError(t, err)
	require.Len(t, dbStorageVolume, 2)
	require.Equal(t, storageVolumeA.Name, dbStorageVolume[0].Name)
	require.Equal(t, storageVolumeB.Name, dbStorageVolume[1].Name)

	// Ensure we have one entry with filter for cluster, server and project
	storageVolumeUUIDs, err = storageVolume.GetAllUUIDsWithFilter(ctx, inventory.StorageVolumeFilter{
		Cluster: ptr.To("one"),
		Server:  ptr.To("one"),
		Project: ptr.To("one"),
	})
	require.NoError(t, err)
	require.Len(t, storageVolumeUUIDs, 1)
	require.ElementsMatch(t, []uuid.UUID{storageVolumeA.UUID}, storageVolumeUUIDs)

	// Ensure we have one entry with filter for cluster, server and project
	dbStorageVolume, err = storageVolume.GetAllWithFilter(ctx, inventory.StorageVolumeFilter{
		Cluster: ptr.To("one"),
		Server:  ptr.To("one"),
		Project: ptr.To("one"),
	})
	require.NoError(t, err)
	require.Len(t, dbStorageVolume, 1)
	require.Equal(t, "one", dbStorageVolume[0].Name)

	// Should get back storageVolumeA unchanged.
	storageVolumeA.Cluster = "one"
	dbStorageVolumeA, err := storageVolume.GetByUUID(ctx, storageVolumeA.UUID)
	require.NoError(t, err)
	require.Equal(t, storageVolumeA, dbStorageVolumeA)

	storageVolumeB.LastUpdated = time.Now().UTC().Truncate(0)
	dbStorageVolumeB, err := storageVolume.UpdateByUUID(ctx, storageVolumeB)
	require.NoError(t, err)
	require.Equal(t, storageVolumeB, dbStorageVolumeB)

	// Delete storage_volumes by ID.
	err = storageVolume.DeleteByUUID(ctx, storageVolumeA.UUID)
	require.NoError(t, err)

	// Delete storage_volumes by cluster Name.
	err = storageVolume.DeleteByClusterName(ctx, "two")
	require.NoError(t, err)

	_, err = storageVolume.GetByUUID(ctx, storageVolumeA.UUID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have no storage_volumes remaining.
	storageVolumeUUIDs, err = storageVolume.GetAllUUIDsWithFilter(ctx, inventory.StorageVolumeFilter{})
	require.NoError(t, err)
	require.Zero(t, storageVolumeUUIDs)
}
