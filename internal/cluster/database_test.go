package cluster_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster"
	clusterRepo "github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
)

func TestFindOrGenCertInfo(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	generated, err := cluster.FindOrGenCertInfo(root, "server.crt", "server.key")
	require.NoError(t, err)

	// The stored keypair is found again.
	found, err := cluster.FindOrGenCertInfo(root, "server.crt", "server.key")
	require.NoError(t, err)
	require.Equal(t, generated.Fingerprint(), found.Fingerprint())

	require.NoError(t, root.WriteFile("server.key", []byte("invalid"), 0o600))
	_, err = cluster.FindOrGenCertInfo(root, "server.crt", "server.key")
	require.Error(t, err)
}

func TestCompletePendingReset(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	// Without the marker, nothing is touched.
	require.NoError(t, root.MkdirAll("database", 0o700))
	require.NoError(t, root.WriteFile("database/local.db", []byte("db"), 0o600))

	completed, err := cluster.CompletePendingReset(t.Context(), root)
	require.NoError(t, err)
	require.False(t, completed)
	require.FileExists(t, filepath.Join(root.Name(), "database/local.db"))

	// With the marker, the database, the cluster certificate and the stored files go, the client keypair is replaced.
	require.NoError(t, root.WriteFile("reset.pending", nil, 0o600))
	require.NoError(t, root.WriteFile("cluster.crt", []byte("cert"), 0o644))
	require.NoError(t, root.WriteFile("cluster.key", []byte("key"), 0o600))
	require.NoError(t, root.MkdirAll("updates/stale", 0o700))
	require.NoError(t, root.WriteFile("updates/stale/update.raw", []byte("update"), 0o600))
	require.NoError(t, root.WriteFile("client.crt", []byte("client"), 0o600))

	completed, err = cluster.CompletePendingReset(t.Context(), root)
	require.NoError(t, err)
	require.True(t, completed)

	for _, name := range []string{"reset.pending", "database", "cluster.crt", "cluster.key", "updates/stale"} {
		require.NoFileExists(t, filepath.Join(root.Name(), name), name)
		require.NoDirExists(t, filepath.Join(root.Name(), name), name)
	}

	clientCert, err := root.ReadFile("client.crt")
	require.NoError(t, err)
	require.Contains(t, string(clientCert), "BEGIN CERTIFICATE")
}

func TestOpenLocalDB_error(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	require.NoError(t, root.WriteFile("database", nil, 0o600))

	_, err = cluster.OpenLocalDB(t.Context(), root)
	require.ErrorContains(t, err, "Failed to create database directory")
}

func TestOpenGlobalDB(t *testing.T) {
	t.Parallel()

	svc := newMembershipService(t)

	globalDB, err := cluster.OpenGlobalDB(t.Context(), svc, false, "")
	require.NoError(t, err)

	t.Cleanup(func() { _ = globalDB.Close() })

	stmts, err := entities.PrepareStmts(globalDB, false)
	require.NoError(t, err)

	ctx := entities.WithPreparedStmts(t.Context(), stmts)
	globalRepo := clusterRepo.NewGlobalRaftRepo(globalDB)
	configRepo := clusterRepo.NewConfigRepo(globalRepo)

	require.NoError(t, svc.Enable(ctx, "", configRepo, globalRepo))
	require.Equal(t, int64(1), globalRepo.GetNodeID())

	// The shared config is stored in the global database.
	_, err = configRepo.GetSharedConfig(ctx)
	require.NoError(t, err)

	merged, err := svc.PersistSharedConfig(ctx, config.SharedChange{})
	require.NoError(t, err)

	stored, err := configRepo.GetSharedConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, merged, *stored)

	require.NoError(t, svc.Stop(context.Background()))
}
