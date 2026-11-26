package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	clusterTx "github.com/cowsql/go-cowsql/cluster/db/transaction"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

// localGlobalDB is a global database knowing the members, whose transactions run on a local database.
type localGlobalDB struct {
	GlobalRaftRepo

	tx    clusterTx.Transactor
	nodes []cowsqldb.NodeInfo
}

func (l *localGlobalDB) GetNodes(context.Context) ([]cowsqldb.NodeInfo, error) { return l.nodes, nil }

func (l *localGlobalDB) DBTX() clusterTx.DBTX { return l.tx.DBTX() }

func (l *localGlobalDB) MaxRetries() int { return l.tx.MaxRetries() }

func (l *localGlobalDB) OnTxStart(exclusive bool) func() { return l.tx.OnTxStart(exclusive) }

func (l *localGlobalDB) TxTimeout() time.Duration { return l.tx.TxTimeout() }

func (l *localGlobalDB) EnterExclusive() error { return l.tx.EnterExclusive() }

// sharedConfigRepo holds the stored shared config.
type sharedConfigRepo struct {
	ConfigRepo

	mu        sync.Mutex
	stored    config.SharedConfig
	gets      int
	getErr    error
	updateErr error
	updated   []config.SharedConfig
}

// errReloadNotTested fails the reload of the shared config on this member, so the shared config of the test binary stays as it is.
var errReloadNotTested = errors.New("The reload on this member is not tested")

func (r *sharedConfigRepo) GetSharedConfig(context.Context) (*config.SharedConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gets++

	// The first read is the one of the revert. Any later read is the reload on this member.
	if r.gets > 1 {
		return nil, errReloadNotTested
	}

	stored := r.stored

	return &stored, r.getErr
}

func (r *sharedConfigRepo) UpdateSharedConfig(_ context.Context, cfg config.SharedConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.updated = append(r.updated, cfg)

	return r.updateErr
}

func TestMembership_PublishSharedConfig(t *testing.T) {
	t.Parallel()

	// The zero change replaces the network section with the empty one.
	change := config.SharedChange{}

	previous := config.SharedConfig{}
	previous.Network.OperationsCenterAddress = "https://192.0.2.100:8443"

	// The stored config still holds the change. The other sections changed meanwhile.
	storedWithChange := config.SharedConfig{}
	storedWithChange.Settings.ServerRegistrationScriptlet = "changed meanwhile"

	reverted := storedWithChange
	reverted.Network = previous.Network

	// Another member changed the network section again.
	storedChangedAgain := config.SharedConfig{}
	storedChangedAgain.Network.OperationsCenterAddress = "https://192.0.2.200:8443"

	tests := []struct {
		name      string
		reloadErr error
		repo      *sharedConfigRepo

		wantUpdated []config.SharedConfig
		wantAsked   []string
		wantGets    int
		wantErr     require.ErrorAssertionFunc
	}{
		{
			name:      "all members apply the change",
			reloadErr: nil,
			repo:      &sharedConfigRepo{stored: storedWithChange},

			wantUpdated: nil,
			wantAsked:   []string{"192.0.2.2:8443"},
			wantGets:    0,
			wantErr:     require.NoError,
		},
		{
			name:      "a member failing to apply the change reverts it",
			reloadErr: boom.Error,
			repo:      &sharedConfigRepo{stored: storedWithChange},

			wantUpdated: []config.SharedConfig{reverted},
			wantAsked:   []string{"192.0.2.2:8443", "192.0.2.2:8443"},
			wantGets:    2,
			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorContains(tt, err, "did not apply the shared config change, it has been reverted", a...)
			},
		},
		{
			name:      "a section changed again by another member is kept",
			reloadErr: boom.Error,
			repo:      &sharedConfigRepo{stored: storedChangedAgain},

			wantUpdated: nil,
			wantAsked:   []string{"192.0.2.2:8443", "192.0.2.2:8443"},
			wantGets:    2,
			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
			},
		},
		{
			name:      "a failed read of the stored config fails the revert",
			reloadErr: boom.Error,
			repo:      &sharedConfigRepo{stored: storedWithChange, getErr: boom.Error},

			wantUpdated: nil,
			wantAsked:   []string{"192.0.2.2:8443"},
			wantGets:    1,
			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorIs(tt, err, boom.Error, a...)
				require.ErrorContains(tt, err, "Failed to revert the shared config", a...)
			},
		},
		{
			name:      "a failed write of the stored config fails the revert",
			reloadErr: boom.Error,
			repo:      &sharedConfigRepo{stored: storedWithChange, updateErr: boom.Error},

			wantUpdated: []config.SharedConfig{reverted},
			wantAsked:   []string{"192.0.2.2:8443"},
			wantGets:    1,
			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorIs(tt, err, boom.Error, a...)
				require.ErrorContains(tt, err, "Failed to revert the shared config", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			db, err := dbdriver.Open(t.TempDir())
			require.NoError(t, err)

			t.Cleanup(func() { _ = db.Close() })

			var global GlobalRaftRepo = &localGlobalDB{
				tx: transaction.Enable(db).(transaction.LocalDB).LocalTransactor(),
				nodes: []cowsqldb.NodeInfo{
					{ID: 1, Name: "member01", Address: "192.0.2.1:8443", Heartbeat: time.Now()},
					{ID: 2, Name: "member02", Address: "192.0.2.2:8443", Heartbeat: time.Now()},
				},
			}

			var repo ConfigRepo = tc.repo

			var mu sync.Mutex
			var asked []string

			m := &membership{
				gateway: &memberGateway{addresses: []string{"192.0.2.2:8443"}},
				reloadConfigOnMember: func(_ context.Context, address string) error {
					mu.Lock()
					defer mu.Unlock()

					asked = append(asked, address)

					return tc.reloadErr
				},
			}

			m.globalRepo.Store(&global)
			m.configRepo.Store(&repo)

			// Run test
			err = m.PublishSharedConfig(t.Context(), change, previous)

			// Assert
			tc.wantErr(t, err, "the error of the publish")
			require.Equal(t, tc.wantUpdated, tc.repo.updated, "the shared config written by the revert")
			require.Equal(t, tc.wantAsked, asked, "the members asked to reload the shared config")
			require.Equal(t, tc.wantGets, tc.repo.gets, "the reads of the stored config by the revert and the local reload")
		})
	}
}
