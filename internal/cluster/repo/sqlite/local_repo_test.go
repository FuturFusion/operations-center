package sqlite_test

import (
	"path/filepath"
	"testing"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/shared/api"
)

// openLocalDB returns the local database of a member in dir.
func openLocalDB(t *testing.T, dir string) *sqlite.LocalDB {
	t.Helper()

	db, err := dbdriver.Open(dir)
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	_, err = dbschema.EnsureLocal(t.Context(), db.DB, dir, dbschema.LocalUpdates(), dbschema.LocalFreshSchema())
	require.NoError(t, err)

	return &sqlite.LocalDB{DB: db.DB}
}

func TestRaftRepo_transactor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	localDB := openLocalDB(t, dir)
	repo := sqlite.NewRaftRepo(localDB, dir)

	require.Equal(t, localDB.DB, repo.DB())
	require.NotNil(t, repo.DBTX())
	require.Zero(t, repo.MaxRetries())
	require.Zero(t, repo.TxTimeout())
	require.Nil(t, repo.OnTxStart(true))
	require.NoError(t, repo.EnterExclusive())
	require.Equal(t, filepath.Join(dir, "global"), repo.GlobalDatabaseDir())
	require.Equal(t, filepath.Join(dir, "global", "logs.db"), repo.LogPath())

	tx, err := localDB.BeginTx(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
}

func TestRaftRepo_clusterAddress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	repo := sqlite.NewRaftRepo(openLocalDB(t, dir), dir)

	address, err := repo.GetClusterAddress(t.Context())
	require.NoError(t, err)
	require.Empty(t, address)

	require.Error(t, repo.SetClusterAddress(t.Context(), "10.0.0.1:8443"))
}

func TestRaftRepo_raftNodes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()
	repo := sqlite.NewRaftRepo(openLocalDB(t, dir), dir)

	// Without a cluster address, the member is the only raft node.
	node, err := repo.DetermineRaftNode(ctx)
	require.NoError(t, err)
	require.Equal(t, cowsqldb.DefaultRaftNode(), node)

	nodes, err := repo.GetRaftNodes(ctx)
	require.NoError(t, err)
	require.Empty(t, nodes)

	require.NoError(t, repo.CreateFirstRaftNode(ctx, "10.0.0.1:8443", "m1"))

	node, found, err := repo.GetRaftNode(ctx, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "10.0.0.1:8443", node.Address)
	require.Equal(t, "m1", node.Name)

	_, found, err = repo.GetRaftNode(ctx, 2)
	require.NoError(t, err)
	require.False(t, found)

	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := certificate.Decode(certPEM)
	require.NoError(t, err)

	require.NoError(t, repo.SetRaftCertificateByAddress(ctx, api.Certificate{Certificate: cert}, "10.0.0.1:8443"))

	// The certificate of a remaining node is kept.
	require.NoError(t, repo.ReplaceRaftNodes(ctx, []cowsqldb.RaftNode{
		{ID: 1, Address: "10.0.0.1:8443", Name: "m1"},
		{ID: 2, Address: "10.0.0.2:8443", Name: "m2"},
	}))

	nodes, err = repo.GetRaftNodes(ctx)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	require.Equal(t, "m2", nodes[1].Name)

	certs, err := repo.GetCertificates(ctx)
	require.NoError(t, err)
	require.Len(t, certs, 1)
	require.True(t, cert.Equal(certs["10.0.0.1:8443"].Certificate))
}
