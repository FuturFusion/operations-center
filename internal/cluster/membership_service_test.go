package cluster_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/repo/mock"
	clusterRepo "github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

// cowsql names the VFS of a node from a counter, which isn't thread safe.
var gatewayCreateMu sync.Mutex

// newMembershipService returns the membership service of a standalone member with its var dir in a new temporary directory.
func newMembershipService(t *testing.T) cluster.MembershipService {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	localDB, err := cluster.OpenLocalDB(t.Context(), root)
	require.NoError(t, err)

	t.Cleanup(func() { _ = localDB.Close() })

	cert, err := cluster.FindOrGenCertInfo(root, "server.crt", "server.key")
	require.NoError(t, err)

	localRepo := clusterRepo.NewRaftRepo(localDB, filepath.Join(root.Name(), "database"))

	gatewayCreateMu.Lock()
	svc, err := cluster.NewClusterMembershipService(t.Context(), root, nil, localRepo, cert, cert, nil)
	gatewayCreateMu.Unlock()
	require.NoError(t, err)

	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	return svc
}

// newSQLDB returns a database for a mocked repo, which the gateway closes on stop.
func newSQLDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := dbdriver.Open(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db.DB
}

// txDBTX is a DBTX already within a transaction.
type txDBTX struct {
	transaction.TX
}

func TestMembership_Enable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		determineNodeIDErr    error
		wantDetermineNodeIDOf string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:               "error - DetermineNodeID",
			determineNodeIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newMembershipService(t)

			db := newSQLDB(t)
			var gotAddress string
			globalRepo := &mock.GlobalRaftRepoMock{
				DBFunc: func() *sql.DB {
					return db
				},
				DetermineNodeIDFunc: func(ctx context.Context, address string) error {
					gotAddress = address
					return tc.determineNodeIDErr
				},
			}

			err := svc.Enable(t.Context(), "10.0.0.1:8443", &mock.ConfigRepoMock{}, globalRepo)
			tc.assertErr(t, err)
			require.Equal(t, "10.0.0.1:8443", gotAddress)
			require.Equal(t, globalRepo, svc.Gateway().Cluster())
		})
	}
}

func TestMembership_PersistSharedConfig(t *testing.T) {
	t.Parallel()

	stored := config.SharedConfig{}
	stored.Settings.ServerRegistrationScriptlet = "stored"

	change := config.SharedChange{}

	tests := []struct {
		name      string
		getErr    error
		updateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:   "error - GetSharedConfig",
			getErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:      "error - UpdateSharedConfig",
			updateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newMembershipService(t)

			db := newSQLDB(t)
			globalRepo := &mock.GlobalRaftRepoMock{
				DBFunc: func() *sql.DB {
					return db
				},
				DBTXFunc: func() transaction.DBTX {
					return txDBTX{}
				},
				DetermineNodeIDFunc: func(ctx context.Context, address string) error {
					return nil
				},
			}

			var updated *config.SharedConfig
			configRepo := &mock.ConfigRepoMock{
				GetSharedConfigFunc: func(ctx context.Context) (*config.SharedConfig, error) {
					return &stored, tc.getErr
				},
				UpdateSharedConfigFunc: func(ctx context.Context, cfg config.SharedConfig) error {
					updated = &cfg
					return tc.updateErr
				},
			}

			require.NoError(t, svc.Enable(t.Context(), "", configRepo, globalRepo))

			merged, err := svc.PersistSharedConfig(t.Context(), change)
			tc.assertErr(t, err)
			if err != nil {
				return
			}

			require.Equal(t, change.Apply(stored), merged)
			require.Equal(t, &merged, updated)
		})
	}
}
