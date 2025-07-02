package sqlite_test

import (
	"context"
	"testing"

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

func TestClusterDatabaseActions(t *testing.T) {
	clusterA := provisioning.Cluster{
		Name:          "one",
		ConnectionURL: "https://cluster-one/",
		Certificate: `-----BEGIN CERTIFICATE-----
cluster A
-----END CERTIFICATE-----
`,
		Status:      api.ClusterStatusReady,
		ServerNames: []string{"server1", "server2"},
	}

	clusterB := provisioning.Cluster{
		Name:          "two",
		ConnectionURL: "https://cluster-one/",
		Certificate: `-----BEGIN CERTIFICATE-----
cluster B
-----END CERTIFICATE-----
`,
		Status:      api.ClusterStatusReady,
		ServerNames: []string{"server10", "server11"},
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

	cluster := sqlite.NewCluster(tx)

	// Add cluster
	_, err = cluster.Create(ctx, clusterA)
	require.NoError(t, err)
	_, err = cluster.Create(ctx, clusterB)
	require.NoError(t, err)

	// Reset ServerNames, since they are only used during Create
	clusterA.ServerNames = nil
	clusterB.ServerNames = nil

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
	clusterA.ID = dbClusterA.ID
	clusterA.LastUpdated = dbClusterA.LastUpdated
	require.Equal(t, clusterA, *dbClusterA)

	dbClusterB, err := cluster.GetByName(ctx, clusterB.Name)
	require.NoError(t, err)
	clusterB.ID = dbClusterB.ID
	clusterB.LastUpdated = dbClusterB.LastUpdated
	require.Equal(t, clusterB, *dbClusterB)

	// Test updating a cluster.
	clusterB.ConnectionURL = "https://foobar.com"
	err = cluster.Update(ctx, clusterB)
	require.NoError(t, err)
	clusterB.Name = "two new"
	err = cluster.Rename(ctx, "two", clusterB.Name)
	require.NoError(t, err)
	dbClusterB, err = cluster.GetByName(ctx, clusterB.Name)
	require.NoError(t, err)
	clusterB.ID = dbClusterB.ID
	clusterB.LastUpdated = dbClusterB.LastUpdated
	require.Equal(t, clusterB, *dbClusterB)

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
	err = cluster.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a cluster that doesn't exist.
	err = cluster.Update(ctx, clusterA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate a cluster.
	_, err = cluster.Create(ctx, clusterB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
