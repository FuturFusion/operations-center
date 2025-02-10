package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerDatabaseActions(t *testing.T) {
	testClusterA := provisioning.Cluster{
		ID:              1,
		Name:            "one",
		ConnectionURL:   "https://cluster-one/",
		ServerHostnames: []string{"one", "two"},
		LastUpdated:     time.Now(),
	}

	testClusterB := provisioning.Cluster{
		ID:              2,
		Name:            "two",
		ConnectionURL:   "https://cluster-two/",
		ServerHostnames: []string{"one", "two"},
		LastUpdated:     time.Now(),
	}

	serverA := provisioning.Server{
		ID:            1,
		ClusterID:     1,
		Hostname:      "one",
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		HardwareData:  incusapi.Resources{},
		VersionData:   json.RawMessage(`{}`),
		LastUpdated:   time.Now(),
	}

	serverB := provisioning.Server{
		ID:            2,
		ClusterID:     2,
		Hostname:      "two",
		Type:          api.ServerTypeMigrationManager,
		ConnectionURL: "https://two/",
		HardwareData:  incusapi.Resources{},
		VersionData:   json.RawMessage(`{}`),
		LastUpdated:   time.Now(),
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

	clusterSvc := provisioning.NewClusterService(sqlite.NewCluster(db))

	server := sqlite.NewServer(db)

	// Cannot add a server with an invalid cluster.
	_, err = server.Create(ctx, serverA)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Add dummy clusters.
	_, err = clusterSvc.Create(ctx, testClusterA)
	require.NoError(t, err)
	_, err = clusterSvc.Create(ctx, testClusterB)
	require.NoError(t, err)

	// Add server
	serverA, err = server.Create(ctx, serverA)
	require.NoError(t, err)
	serverB, err = server.Create(ctx, serverB)
	require.NoError(t, err)

	// Ensure we have three entries
	servers, err := server.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, servers, 2)

	serverIDs, err := server.GetAllHostnames(ctx)
	require.NoError(t, err)
	require.Len(t, serverIDs, 2)
	require.ElementsMatch(t, []string{"one", "two"}, serverIDs)

	// Should get back serverA unchanged.
	dbServerA, err := server.GetByID(ctx, serverA.ID)
	require.NoError(t, err)
	require.Equal(t, serverA, dbServerA)

	dbServerA, err = server.GetByHostname(ctx, serverA.Hostname)
	require.NoError(t, err)
	require.Equal(t, serverA, dbServerA)

	// Test updating a server.
	serverB.ClusterID = 2
	dbServerB, err := server.UpdateByID(ctx, serverB)
	require.NoError(t, err)
	require.Equal(t, serverB, dbServerB)
	dbServerB, err = server.GetByID(ctx, serverB.ID)
	require.NoError(t, err)
	require.Equal(t, serverB, dbServerB)

	// Delete a server.
	err = server.DeleteByID(ctx, serverA.ID)
	require.NoError(t, err)
	_, err = server.GetByID(ctx, serverA.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have two servers remaining.
	servers, err = server.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	// Can't delete a server that doesn't exist.
	err = server.DeleteByID(ctx, 3)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a server that doesn't exist.
	_, err = server.UpdateByID(ctx, serverA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate a server.
	_, err = server.Create(ctx, serverB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Ensure deletion of cluster fails if a linked server is present.
	err = clusterSvc.DeleteByName(ctx, "two")
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
