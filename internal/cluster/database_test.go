package cluster_test

import (
	"context"
	"os"
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
