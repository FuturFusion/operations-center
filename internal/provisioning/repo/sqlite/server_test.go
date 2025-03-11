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
		Name:            "one",
		ConnectionURL:   "https://cluster-one/",
		ServerHostnames: []string{"one", "two"},
		LastUpdated:     time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	testClusterB := provisioning.Cluster{
		Name:            "two",
		ConnectionURL:   "https://cluster-two/",
		ServerHostnames: []string{"one", "two"},
		LastUpdated:     time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	serverA := provisioning.Server{
		Cluster:       "one",
		Name:          "one",
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		HardwareData:  incusapi.Resources{},
		VersionData:   json.RawMessage(nil),
		LastUpdated:   time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	serverB := provisioning.Server{
		Cluster:       "two",
		Name:          "two",
		Type:          api.ServerTypeMigrationManager,
		ConnectionURL: "https://two/",
		HardwareData:  incusapi.Resources{},
		VersionData:   json.RawMessage(nil),
		LastUpdated:   time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
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

	clusterSvc := provisioning.NewClusterService(sqlite.NewCluster(db), nil)

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
	_, err = server.Create(ctx, serverA)
	require.NoError(t, err)
	_, err = server.Create(ctx, serverB)
	require.NoError(t, err)

	// Ensure we have two entries
	servers, err := server.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, servers, 2)

	serverIDs, err := server.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, serverIDs, 2)
	require.ElementsMatch(t, []string{"one", "two"}, serverIDs)

	// Should get back serverA unchanged.
	dbServerA, err := server.GetByName(ctx, serverA.Name)
	require.NoError(t, err)
	require.Equal(t, serverA, dbServerA)

	dbServerA, err = server.GetByName(ctx, serverA.Name)
	require.NoError(t, err)
	require.Equal(t, serverA, dbServerA)

	// Test updating a server.
	serverB.Cluster = "two"
	dbServerB, err := server.UpdateByName(ctx, serverB.Name, serverB)
	require.NoError(t, err)
	require.Equal(t, serverB, dbServerB)
	dbServerB, err = server.GetByName(ctx, serverB.Name)
	require.NoError(t, err)
	require.Equal(t, serverB, dbServerB)

	// Delete a server.
	err = server.DeleteByName(ctx, serverA.Name)
	require.NoError(t, err)
	_, err = server.GetByName(ctx, serverA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have two servers remaining.
	servers, err = server.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	// Can't delete a server that doesn't exist.
	err = server.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a server that doesn't exist.
	_, err = server.UpdateByName(ctx, serverA.Name, serverA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate a server.
	_, err = server.Create(ctx, serverB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Ensure deletion of cluster fails if a linked server is present.
	err = clusterSvc.DeleteByName(ctx, "two")
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
