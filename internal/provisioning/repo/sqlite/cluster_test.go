package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func TestClusterDatabaseActions(t *testing.T) {
	clusterA := provisioning.Cluster{
		Name:            "one",
		ConnectionURL:   "https://cluster-one/",
		ServerHostnames: []string{"server1", "server2"},
		LastUpdated:     time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
	}

	clusterB := provisioning.Cluster{
		Name:            "two",
		ConnectionURL:   "https://cluster-one/",
		ServerHostnames: []string{"server10", "server11"},
		LastUpdated:     time.Now().UTC().Truncate(0), // Truncate to remove the monotonic clock.
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

	cluster, err := sqlite.NewCluster(db)
	require.NoError(t, err)

	// Add cluster
	_, err = cluster.Create(ctx, clusterA)
	require.NoError(t, err)
	_, err = cluster.Create(ctx, clusterB)
	require.NoError(t, err)

	// Ensure we have two entries
	clusters, err := cluster.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, clusters, 2)

	clusterIDs, err := cluster.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, clusterIDs, 2)
	require.ElementsMatch(t, []string{"one", "two"}, clusterIDs)

	// Should get back clusterA unchanged.
	dbClusterA, err := cluster.GetByName(ctx, clusterA.Name)
	require.NoError(t, err)
	require.Equal(t, clusterA, dbClusterA)

	dbClusterA, err = cluster.GetByName(ctx, clusterA.Name)
	require.NoError(t, err)
	require.Equal(t, clusterA, dbClusterA)

	// Test updating a cluster.
	clusterB.ServerHostnames = []string{"server100"}
	dbClusterB, err := cluster.UpdateByName(ctx, clusterB.Name, clusterB)
	require.NoError(t, err)
	require.Equal(t, clusterB, dbClusterB)
	dbClusterB, err = cluster.GetByName(ctx, clusterB.Name)
	require.NoError(t, err)
	require.Equal(t, clusterB, dbClusterB)

	// Delete a cluster.
	err = cluster.DeleteByName(ctx, clusterA.Name)
	require.NoError(t, err)
	_, err = cluster.GetByName(ctx, clusterA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have two clusters remaining.
	clusters, err = cluster.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, clusters, 1)

	// Can't delete a cluster that doesn't exist.
	err = cluster.DeleteByName(ctx, clusterA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a cluster that doesn't exist.
	_, err = cluster.UpdateByName(ctx, clusterA.Name, clusterA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate a cluster.
	_, err = cluster.Create(ctx, clusterB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
