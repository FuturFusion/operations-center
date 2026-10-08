package server_test

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

// pollDBReadTimeout is how long a read of an unrelated server record may take
// while a poll is talking to a BMC.
const pollDBReadTimeout = 5 * time.Second

// TestServerService_PollServerDoesNotHoldTheDatabaseAcrossABMCCall asserts, that
// polling an offline server, whose BMC is slow to answer, leaves the database
// alone meanwhile.
//
// The daemon runs on a single SQLite connection, so a transaction, that spans a
// BMC call, blocks every other reader and writer, including the ones asking
// about a completely different server.
func TestServerService_PollServerDoesNotHoldTheDatabaseAcrossABMCCall(t *testing.T) {
	ctx := t.Context()

	serverDB := newPollTestServerRepo(t)

	var err error

	unresponsive := deploymentTestServer("unresponsive")
	unresponsive.Status = api.ServerStatusOffline
	unresponsive.StatusDetail = api.ServerStatusDetailOfflineUnresponsive
	unresponsive.BMCData = deploymentTestBMCData(deploymentTestOpticalMedia)

	unresponsive.ID, err = serverDB.Create(ctx, unresponsive)
	require.NoError(t, err)
	require.NoError(t, serverDB.Update(ctx, unresponsive))

	bystander := deploymentTestServer("bystander")

	_, err = serverDB.Create(ctx, bystander)
	require.NoError(t, err)

	entered := make(chan struct{})
	release := make(chan struct{})

	bmc := &adapterMock.BMCServerClientPortMock{
		GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
			close(entered)

			select {
			case <-release:
			case <-ctx.Done():
				return api.BMCData{}, ctx.Err()
			}

			return deploymentTestBMCData(deploymentTestOpticalMedia), nil
		},
	}

	serverClient := &adapterMock.ServerClientPortMock{
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			return domain.NewRetryableErr(boom.Error)
		},
	}

	serverSvc := provisioningServer.New(serverDB, serverClient, nil, nil, nil, nil, nil, tls.Certificate{},
		provisioningServer.WithNow(time.Now),
		provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmc),
	)

	// Run test
	polled := make(chan error, 1)

	go func() {
		polled <- serverSvc.PollServer(context.Background(), unresponsive, true)
	}()

	<-entered

	read := make(chan error, 1)

	go func() {
		_, err := serverDB.GetByName(context.Background(), bystander.Name)
		read <- err
	}()

	// Assert
	select {
	case err := <-read:
		require.NoError(t, err)

	case <-time.After(pollDBReadTimeout):
		require.FailNow(t, "reading an unrelated server blocked while the poll was talking to a BMC")
	}

	close(release)

	require.NoError(t, <-polled)
}

func TestServerService_PollServerPersistsRegistrationScriptletResult(t *testing.T) {
	tests := []struct {
		name string

		scriptlet func(t *testing.T, serverDB provisioning.ServerRepo, server *provisioning.Server) error

		assertErr              require.ErrorAssertionFunc
		wantServerNames        []string
		wantName               string
		wantDescription        string
		wantProperties         api.ConfigMap
		wantServerStatus       api.ServerStatus
		wantServerStatusDetail api.ServerStatusDetail
	}{
		{
			name: "success - rename",
			scriptlet: func(t *testing.T, serverDB provisioning.ServerRepo, server *provisioning.Server) error {
				t.Helper()

				server.Name = "renamed"
				server.Description = "set by scriptlet"

				return nil
			},

			assertErr:              require.NoError,
			wantServerNames:        []string{"renamed", "taken"},
			wantName:               "renamed",
			wantDescription:        "set by scriptlet",
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - update during the scriptlet run is kept",
			scriptlet: func(t *testing.T, serverDB provisioning.ServerRepo, server *provisioning.Server) error {
				t.Helper()

				current, err := serverDB.GetByName(t.Context(), server.Name)
				require.NoError(t, err, "reading the server during the scriptlet run must not fail")

				current.Description = "set by operator"

				err = serverDB.Update(t.Context(), *current)
				require.NoError(t, err, "updating the server during the scriptlet run must not fail")

				server.Properties = api.ConfigMap{"rack": "1"}

				return nil
			},

			assertErr:              require.NoError,
			wantServerNames:        []string{"pending", "taken"},
			wantName:               "pending",
			wantDescription:        "set by operator",
			wantProperties:         api.ConfigMap{"rack": "1"},
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - changes of a failed scriptlet are not persisted",
			scriptlet: func(t *testing.T, serverDB provisioning.ServerRepo, server *provisioning.Server) error {
				t.Helper()

				server.Name = "renamed"
				server.Description = "set by scriptlet"

				return boom.Error
			},

			assertErr:              require.NoError,
			wantServerNames:        []string{"pending", "taken"},
			wantName:               "pending",
			wantDescription:        "",
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "error - rename to an existing name keeps the server pending for a retry",
			scriptlet: func(t *testing.T, serverDB provisioning.ServerRepo, server *provisioning.Server) error {
				t.Helper()

				server.Name = "taken"
				server.Description = "set by scriptlet"

				return nil
			},

			assertErr:              require.Error,
			wantServerNames:        []string{"pending", "taken"},
			wantName:               "pending",
			wantDescription:        "",
			wantServerStatus:       api.ServerStatusPending,
			wantServerStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			ctx := t.Context()

			serverDB := newPollTestServerRepo(t)

			pending := deploymentTestServer("pending")
			pending.Status = api.ServerStatusPending
			pending.StatusDetail = api.ServerStatusDetailPendingRegistering

			var err error

			pending.ID, err = serverDB.Create(ctx, pending)
			require.NoError(t, err, "creating the pending server must not fail")

			_, err = serverDB.Create(ctx, deploymentTestServer("taken"))
			require.NoError(t, err, "creating the second server must not fail")

			serverClient := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			runner := &adapterMock.ServerScriptletPortMock{
				ServerRegistrationRunFunc: func(ctx context.Context, server *provisioning.Server) error {
					require.False(t, transaction.IsActive(ctx), "the scriptlet must run outside of a transaction")

					return tc.scriptlet(t, serverDB, server)
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return nil, nil
				},
			}

			serverSvc := provisioningServer.New(serverDB, serverClient, runner, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(time.Now),
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.PollServer(ctx, pending, false)

			// Assert
			tc.assertErr(t, err)

			serverNames, err := serverDB.GetAllNames(ctx)
			require.NoError(t, err, "listing the server names must not fail")
			require.ElementsMatch(t, tc.wantServerNames, serverNames, "a rename must not leave the previous name behind")

			server, err := serverDB.GetByName(ctx, tc.wantName)
			require.NoError(t, err, "the server must exist under the expected name")
			require.Equal(t, pending.ID, server.ID, "a rename must keep the ID of the server")
			require.Equal(t, tc.wantDescription, server.Description)
			require.Equal(t, tc.wantProperties, server.Properties)
			require.Equal(t, tc.wantServerStatus, server.Status)
			require.Equal(t, tc.wantServerStatusDetail, server.StatusDetail)
		})
	}
}

func TestServerService_PollServerRunsRegistrationScriptletOnce(t *testing.T) {
	tests := []struct {
		name string

		prepare   func(t *testing.T, serverDB provisioning.ServerRepo, pending provisioning.Server)
		scriptlet func(t *testing.T, poll func() error) error

		assertErr          require.ErrorAssertionFunc
		wantScriptletCalls int
	}{
		{
			name: "registration completed by another poll",
			prepare: func(t *testing.T, serverDB provisioning.ServerRepo, pending provisioning.Server) {
				t.Helper()

				pending.Status = api.ServerStatusReady
				pending.StatusDetail = api.ServerStatusDetailNone

				err := serverDB.Update(t.Context(), pending)
				require.NoError(t, err, "completing the registration must not fail")
			},
			scriptlet: func(t *testing.T, poll func() error) error {
				t.Helper()

				return nil
			},

			assertErr:          require.NoError,
			wantScriptletCalls: 0,
		},
		{
			name: "second poll during the scriptlet run",
			prepare: func(t *testing.T, serverDB provisioning.ServerRepo, pending provisioning.Server) {
				t.Helper()
			},
			scriptlet: func(t *testing.T, poll func() error) error {
				t.Helper()

				err := poll()
				require.True(t, domain.IsRetryableError(err), "a poll during the scriptlet run must ask for a retry")

				return nil
			},

			assertErr:          require.NoError,
			wantScriptletCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			ctx := t.Context()

			serverDB := newPollTestServerRepo(t)

			pending := deploymentTestServer("pending")
			pending.Status = api.ServerStatusPending
			pending.StatusDetail = api.ServerStatusDetailPendingRegistering

			var err error

			pending.ID, err = serverDB.Create(ctx, pending)
			require.NoError(t, err, "creating the pending server must not fail")

			tc.prepare(t, serverDB, pending)

			serverClient := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			var serverSvc provisioning.ServerService

			runner := &adapterMock.ServerScriptletPortMock{
				ServerRegistrationRunFunc: func(ctx context.Context, server *provisioning.Server) error {
					return tc.scriptlet(t, func() error {
						return serverSvc.PollServer(ctx, pending, false)
					})
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return nil, nil
				},
			}

			serverSvc = provisioningServer.New(serverDB, serverClient, runner, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(time.Now),
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.PollServer(ctx, pending, false)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, runner.ServerRegistrationRunCalls(), tc.wantScriptletCalls)
		})
	}
}

func newPollTestServerRepo(t *testing.T) provisioning.ServerRepo {
	t.Helper()

	tmpDir := t.TempDir()

	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = dbschema.Ensure(t.Context(), db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)

	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	return sqlite.NewServer(tx)
}
