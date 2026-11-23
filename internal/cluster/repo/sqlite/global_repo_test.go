package sqlite_test

import (
	"net/http"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestGlobalRaftRepo_transactor(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	repo := sqlite.NewGlobalRaftRepo(db)

	require.Equal(t, db.DB, repo.DB())
	require.NotNil(t, repo.DBTX())
	require.Positive(t, repo.MaxRetries())
	require.Positive(t, repo.TxTimeout())
	require.Equal(t, db.SchemaVersion(), repo.SchemaVersion())

	// The threshold is taken from the cluster config.
	threshold, err := repo.GetNodeOfflineThreshold(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Duration(config.GetCluster().OfflineThresholdSeconds)*time.Second, threshold)

	// All members are in the default failure domain.
	domains, err := repo.GetNodesFailureDomains(t.Context())
	require.NoError(t, err)
	require.Empty(t, domains)
}

func TestGlobalRaftRepo_DetermineNodeID(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	ctx := t.Context()
	repo := sqlite.NewGlobalRaftRepo(db)

	// A standalone member is the first member, whatever its address.
	require.NoError(t, repo.DetermineNodeID(ctx, "10.0.0.9:8443"))
	require.Equal(t, int64(1), repo.GetNodeID())

	require.NoError(t, repo.BootstrapNode(ctx, "m1", "10.0.0.1:8443"))
	_, err = repo.CreateNode(ctx, "m2", "10.0.0.2:8443", 0)
	require.NoError(t, err)

	require.NoError(t, repo.DetermineNodeID(ctx, "10.0.0.2:8443"))
	require.Equal(t, int64(2), repo.GetNodeID())

	require.ErrorContains(t, repo.DetermineNodeID(ctx, "10.0.0.9:8443"), "No node registered with address")
}

func TestGlobalRaftRepo_members(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	ctx := t.Context()
	repo := sqlite.NewGlobalRaftRepo(db)

	count, err := repo.GetNodesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	require.NoError(t, repo.BootstrapNode(ctx, "m1", "10.0.0.1:8443"))

	m1, err := repo.GetNodeByName(ctx, "m1", false)
	require.NoError(t, err)
	require.Equal(t, int64(1), m1.ID)
	require.Equal(t, "10.0.0.1:8443", m1.Address)

	// A joining member is pending until it has joined.
	id, err := repo.CreateNode(ctx, "m2", "10.0.0.2:8443", 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), id)
	require.NoError(t, repo.SetNodePendingFlag(ctx, id, true))

	_, err = repo.GetNodeByAddress(ctx, "10.0.0.2:8443", false)
	require.True(t, api.StatusErrorCheck(err, http.StatusNotFound))

	m2, err := repo.GetNodeByAddress(ctx, "10.0.0.2:8443", true)
	require.NoError(t, err)
	require.Equal(t, "m2", m2.Name)

	nodes, err := repo.GetNodes(ctx)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	require.NoError(t, repo.SetNodePendingFlag(ctx, id, false))

	nodes, err = repo.GetNodes(ctx)
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	_, err = repo.GetNodeByName(ctx, "m3", false)
	require.True(t, api.StatusErrorCheck(err, http.StatusNotFound))

	heartbeat := time.Now().UTC()
	require.NoError(t, repo.SetNodeHeartbeat(ctx, "10.0.0.2:8443", heartbeat))

	m2, err = repo.GetNodeByName(ctx, "m2", false)
	require.NoError(t, err)
	require.WithinDuration(t, heartbeat, m2.Heartbeat, time.Second)

	err = repo.SetNodeHeartbeat(ctx, "10.0.0.3:8443", heartbeat)
	require.True(t, api.StatusErrorCheck(err, http.StatusNotFound))

	require.NoError(t, repo.RemoveNode(ctx, "m2"))
	require.Error(t, repo.RemoveNode(ctx, "m2"))

	count, err = repo.GetNodesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestGlobalRaftRepo_NodeIsOutdated(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	ctx := t.Context()
	repo := sqlite.NewGlobalRaftRepo(db)

	// The local member must be known.
	_, err = repo.NodeIsOutdated(ctx)
	require.ErrorContains(t, err, "local member not found")

	require.NoError(t, repo.BootstrapNode(ctx, "m1", "10.0.0.1:8443"))
	_, err = repo.CreateNode(ctx, "m2", "10.0.0.2:8443", 0)
	require.NoError(t, err)
	require.NoError(t, repo.DetermineNodeID(ctx, "10.0.0.1:8443"))

	outdated, err := repo.NodeIsOutdated(ctx)
	require.NoError(t, err)
	require.False(t, outdated)

	// Another member runs a newer schema.
	_, err = db.ExecContext(ctx, `UPDATE cluster_members SET schema = schema + 1 WHERE name = 'm2'`)
	require.NoError(t, err)

	outdated, err = repo.NodeIsOutdated(ctx)
	require.NoError(t, err)
	require.True(t, outdated)
}

func TestGlobalRaftRepo_certificates(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	ctx := t.Context()
	repo := sqlite.NewGlobalRaftRepo(db)

	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := certificate.Decode(certPEM)
	require.NoError(t, err)

	// The standalone member is not listed.
	require.NoError(t, repo.SetNodeCertificateByName(ctx, "none", cert))

	certs, err := repo.GetCertificates(ctx)
	require.NoError(t, err)
	require.Empty(t, certs)

	require.NoError(t, repo.BootstrapNode(ctx, "m1", "10.0.0.1:8443"))

	certs, err = repo.GetCertificates(ctx)
	require.NoError(t, err)
	require.Len(t, certs, 1)
	require.True(t, cert.Equal(certs["10.0.0.1:8443"].Certificate))

	require.Error(t, repo.SetNodeCertificateByName(ctx, "m2", cert))
}
