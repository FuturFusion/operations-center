package sqlite_test

import (
	"context"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
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

func TestClusterDatabaseActions(t *testing.T) {
	certPEMA, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprintA, err := incustls.CertFingerprintStr(string(certPEMA))
	require.NoError(t, err)

	certPEMB, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprintB, err := incustls.CertFingerprintStr(string(certPEMB))
	require.NoError(t, err)

	clusterA := provisioning.Cluster{
		Name:          "one",
		ConnectionURL: "https://cluster-one/",
		Certificate:   ptr.To(string(certPEMA)),
		Fingerprint:   fingerprintA,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"server1", "server2"},
		Channel:       "stable",
	}

	clusterB := provisioning.Cluster{
		Name:          "two",
		ConnectionURL: "https://cluster-one/",
		Certificate:   ptr.To(string(certPEMB)),
		Fingerprint:   fingerprintB,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"server10", "server11"},
		Channel:       "stable",
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

	// Should have one clusters remaining.
	clusters, err = cluster.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, clusters, 1)

	// Can't delete a cluster that doesn't exist.
	err = cluster.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a cluster that doesn't exist.
	err = cluster.Update(ctx, clusterA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate cluster.
	_, err = cluster.Create(ctx, clusterB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}

func TestClusterNullCert(t *testing.T) {
	clusterCertNull1 := provisioning.Cluster{
		Name:          "one",
		ConnectionURL: "https://cluster-one/",
		Certificate:   nil,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"server1", "server2"},
		Channel:       "stable",
	}

	clusterCertNull2 := provisioning.Cluster{
		Name:          "two",
		ConnectionURL: "https://cluster-one/",
		Certificate:   nil,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"server10", "server11"},
		Channel:       "stable",
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
	_, err = cluster.Create(ctx, clusterCertNull1)
	require.NoError(t, err)
	_, err = cluster.Create(ctx, clusterCertNull2)
	require.NoError(t, err)

	// Ensure we have two entries
	clusters, err := cluster.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, clusters, 2)
}
