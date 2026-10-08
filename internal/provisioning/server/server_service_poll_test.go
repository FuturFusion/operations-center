package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerService_PollServers(t *testing.T) {
	tests := []struct {
		name                        string
		repoGetAllWithFilterServers provisioning.Servers
		repoGetAllWithFilterErr     error
		repoGetByNameErr            queue.Errs
		clientPingErr               error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                        "success - no pending servers",
			repoGetAllWithFilterServers: provisioning.Servers{},

			assertErr: require.NoError,
		},
		{
			name:                    "error - GetAllWithFilter",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client Ping",
			repoGetAllWithFilterServers: provisioning.Servers{
				{
					Name:   "one",
					Status: api.ServerStatusPending,
				},
				{
					Name:   "two",
					Status: api.ServerStatusReady,
				},
			},
			repoGetByNameErr: queue.Errs{
				boom.Error,
				domain.NewRetryableErr(boom.Error),
			},
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repoMock.ServerRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilterServers, tc.repoGetAllWithFilterErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return nil, tc.repoGetByNameErr.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err := serverSvc.PollServers(t.Context(), provisioning.ServerFilter{
				Status: new(api.ServerStatusPending),
			}, true)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_PollServer_connectionTestWithCertificateUpdate(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	httpsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	httpsServer.StartTLS()
	defer httpsServer.Close()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name                   string
		serverArg              provisioning.Server
		clientPing             []queue.Item[struct{}]
		repoGetByName          []queue.Item[*provisioning.Server]
		repoUpdate             []queue.Item[struct{}]
		clusterSvcGetByName    *provisioning.Cluster
		clusterSvcGetByNameErr error
		clusterSvcUpdateErr    error

		assertErr               require.ErrorAssertionFunc
		assertLog               log.MatcherFunc
		assertServerCertificate string
		wantServerStatus        api.ServerStatus
		wantLastSeen            time.Time
	}{
		{
			name: "success",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusPending,
						Certificate: new(`-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`),
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			assertServerCertificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
			wantServerStatus: api.ServerStatusReady,
			wantLastSeen:     fixedDate,
		},
		{
			name: "error - client Ping - server state unknown",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusUnknown,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusUnknown,
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog: log.Match("Server connection test failed"),
		},
		{
			name: "error - client Ping - server state pending",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusPending,
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        errassert.RetryableBoomError,
			assertLog:        log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus: api.ServerStatusPending,
		},
		{
			name: "error - client Ping - server state offline rebooting",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:         "one",
						Status:       api.ServerStatusOffline,
						StatusDetail: api.ServerStatusDetailOfflineRebooting,
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        errassert.RetryableBoomError,
			assertLog:        log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - client Ping - server state offline shutdown",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:         "one",
						Status:       api.ServerStatusOffline,
						StatusDetail: api.ServerStatusDetailOfflineShutdown,
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog:        log.Match("Server connection test failed.*shut down"),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - client Ping - server state offline unresponsive",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:         "one",
						Status:       api.ServerStatusOffline,
						StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog:        log.Match("Server connection test failed.*unresponsive"),
			wantServerStatus: api.ServerStatusOffline,
		},

		{
			name: "error - client Ping with tls.CertificateVerificationError but server is not part of cluster",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusPending,
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr:        errassert.RetryableErrorContains("failed to verify certificate"),
			assertLog:        log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus: api.ServerStatusPending,
		},
		{
			name: "success - cluster now has publicly valid certificate",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Certificate: new(`-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`),
				Cluster:            new("cluster"),
				ClusterCertificate: new("certificate"),
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "one",
						Certificate: new(`-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`),
					},
				},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
				{},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clusterSvcGetByName: &provisioning.Cluster{
				Name:        "cluster",
				Certificate: new("certificate"),
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			assertServerCertificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
			wantServerStatus: api.ServerStatusReady,
			wantLastSeen:     fixedDate,
		},
		{
			name: "error - client Ping with tls.CertificateVerificationError but second ping fails",
			serverArg: provisioning.Server{
				Name:               "one",
				Status:             api.ServerStatusReady,
				Cluster:            new("cluster"),
				ClusterCertificate: new("certificate"),
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
				{
					Err: boom.Error,
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{
				Name:        "cluster",
				Certificate: new("certificate"),
			},

			assertErr:        require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog:        log.Match("Server connection test failed"),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - cluster now has publicly valid certificate - clusterSvc.GetByName",
			serverArg: provisioning.Server{
				Name:               "one",
				Status:             api.ServerStatusReady,
				Cluster:            new("cluster"),
				ClusterCertificate: new("certificate"),
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
				{},
			},
			clusterSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - cluster now has publicly valid certificate - clusterSvc.Update",
			serverArg: provisioning.Server{
				Name:               "one",
				Status:             api.ServerStatusReady,
				Cluster:            new("cluster"),
				ClusterCertificate: new("certificate"),
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
				{},
			},
			clusterSvcGetByName: &provisioning.Cluster{
				Name:        "cluster",
				Certificate: new("certificate"),
			},
			clusterSvcUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},

		{
			name: "success - standalone server now has publicly valid certificate",
			serverArg: provisioning.Server{
				Name: "one",
				Certificate: new(`-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`),
				Status:              api.ServerStatusReady,
				Type:                api.ServerTypeMigrationManager,
				ConnectionURL:       "https:/127.0.0.1:7443",
				PublicConnectionURL: httpsServer.URL,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
					},
				},
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
						Certificate: new(string(
							pem.EncodeToMemory(
								&pem.Block{
									Type:  "CERTIFICATE",
									Bytes: httpsServer.TLS.Certificates[0].Leaf.Raw,
								},
							),
						)),
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
				{},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because
				// standalone server now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			assertServerCertificate: func() string {
				return string(
					pem.EncodeToMemory(
						&pem.Block{
							Type:  "CERTIFICATE",
							Bytes: httpsServer.TLS.Certificates[0].Leaf.Raw,
						},
					),
				)
			}(),
			wantServerStatus: api.ServerStatusReady,
			wantLastSeen:     fixedDate,
		},
		{
			name: "error - standalone server - invalid public connection URL",
			serverArg: provisioning.Server{
				Name:                "one",
				Status:              api.ServerStatusReady,
				Type:                api.ServerTypeMigrationManager,
				ConnectionURL:       "https:/127.0.0.1:7443",
				PublicConnectionURL: ":|\\", // invalid
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because
				// standalone server now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr:        require.NoError,
			assertLog:        log.Match("Server connection test failed"),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - standalone server - connection error",
			serverArg: provisioning.Server{
				Name:                "one",
				Status:              api.ServerStatusReady,
				Type:                api.ServerTypeMigrationManager,
				ConnectionURL:       "https:/127.0.0.1:7443",
				PublicConnectionURL: "https:/127.0.0.1:7443",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because
				// standalone server now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr:        require.NoError,
			assertLog:        log.Match("(?ms)Refresh certificate connection attempt to public connection URL failed.*Server connection test failed"),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - standalone server - connection error not TLS",
			serverArg: provisioning.Server{
				Name:                "one",
				Status:              api.ServerStatusReady,
				Type:                api.ServerTypeMigrationManager,
				ConnectionURL:       "https:/127.0.0.1:7443",
				PublicConnectionURL: httpServer.URL,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because
				// standalone server now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr:        require.NoError,
			assertLog:        log.Match("(?ms)Refresh certificate connection attempt did not return TLS connection or no peer certificates.*Server connection test failed"),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - standalone server - connection error not TLS - repo.Update error",
			serverArg: provisioning.Server{
				Name:                "one",
				Status:              api.ServerStatusReady,
				Type:                api.ServerTypeMigrationManager,
				ConnectionURL:       "https:/127.0.0.1:7443",
				PublicConnectionURL: httpServer.URL,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:   "one",
						Status: api.ServerStatusReady,
					},
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because
				// standalone server now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr:        boom.ErrorIs,
			assertLog:        log.Match("(?ms)Refresh certificate connection attempt did not return TLS connection or no peer certificates.*Server connection test failed"),
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name: "error - standalone server - repo.GetByName",
			serverArg: provisioning.Server{
				Name: "one",
				Certificate: new(`-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`),
				Status:              api.ServerStatusReady,
				Type:                api.ServerTypeMigrationManager,
				ConnectionURL:       "https:/127.0.0.1:7443",
				PublicConnectionURL: httpsServer.URL,
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},
			clientPing: []queue.Item[struct{}]{
				// Simulate failing connection with pinned certificate, because
				// standalone server now has a publicly valid certificate (e.g. ACME).
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					require.Equal(t, tc.wantServerStatus, server.Status)
					require.Equal(t, tc.wantLastSeen, server.LastSeen)
					require.Equal(t, tc.assertServerCertificate, ptr.From(server.Certificate))
					_, err := queue.Pop(t, &tc.repoUpdate)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					_, err := queue.Pop(t, &tc.clientPing)
					return err
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			runner := &adapterMock.ServerScriptletPortMock{
				ServerRegistrationRunFunc: func(ctx context.Context, server *provisioning.Server) error {
					return nil
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.clusterSvcGetByName, tc.clusterSvcGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster, updateServers bool) error {
					return tc.clusterSvcUpdateErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, runner, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithHTTPClient(httpsServer.Client()),
			)
			serverSvc.SetClusterService(clusterSvc)

			// Run test
			err = serverSvc.PollServer(context.Background(), tc.serverArg, false)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.clientPing)
			require.Empty(t, tc.repoGetByName)
			require.Empty(t, tc.repoUpdate)
		})
	}
}

func TestServerService_PollServer(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                           string
		serverArg                      provisioning.Server
		updateServerConfigArg          bool
		clientIsReadyErr               error
		clientGetResourcesErr          error
		clientGetOSData                api.OSData
		clientGetOSDataErr             error
		clientGetVersionData           api.ServerVersionData
		clientGetVersionDataErr        error
		runnerServerRegistrationRunErr error
		repoGetByName                  *provisioning.Server
		repoGetByNameErr               error
		clusterSvcGetByName            *provisioning.Cluster
		clusterSvcGetByNameErr         error
		repoUpdateErr                  error
		updateSvcGetAllWithFilter      provisioning.Updates
		updateSvcGetAllWithFilterErr   error

		assertErr                 require.ErrorAssertionFunc
		assertLog                 log.MatcherFunc
		wantServerStatusDetail    *api.ServerStatusDetail
		wantServerVersionData     *api.ServerVersionData
		wantServerConnectionURL   *string
		wantServerTriggeredUpdate *provisioning.ServerTriggeredUpdate
	}{
		{
			name: "success",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - without config update",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: false,
			repoGetByName: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - updating",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingOS,
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - updating, update has been applied",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingOS,
				Channel:      "stable",
				VersionData:  pollServerVersionData("1"),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerVersionData("2"),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailNone),
		},
		{
			name: "success - updating application, update has been applied",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingApplication,
				Channel:      "stable",
				VersionData:  pollServerVersionData("1"),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerVersionData("2"),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailNone),
		},
		{
			// The update has been triggered for the application only, so the OS being
			// out of date must not keep the server in the updating state.
			name: "success - updating application, triggered application is up to date while the OS is not",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingApplication,
				Channel:      "stable",
				StatusInternal: provisioning.ServerStatusInternal{
					Update: &provisioning.ServerUpdate{
						Triggered: &provisioning.ServerTriggeredUpdate{
							Applications: map[string]string{
								"incus": "2",
							},
							TriggeredAt: fixedDate,
						},
					},
				},
				VersionData: pollServerApplicationVersionData("1"),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerApplicationVersionData("2"),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailNone),
		},
		{
			name: "success - updating application, triggered application is still pending",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingApplication,
				Channel:      "stable",
				StatusInternal: provisioning.ServerStatusInternal{
					Update: &provisioning.ServerUpdate{
						Triggered: &provisioning.ServerTriggeredUpdate{
							Applications: map[string]string{
								"incus": "2",
							},
							TriggeredAt: fixedDate,
						},
					},
				},
				VersionData: pollServerApplicationVersionData("1"),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerApplicationVersionData("1"),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailReadyUpdatingApplication),
			wantServerTriggeredUpdate: &provisioning.ServerTriggeredUpdate{
				Applications: map[string]string{
					"incus": "2",
				},
				TriggeredAt: fixedDate,
			},
		},
		{
			name: "success - updating, update is still pending",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingOS,
				Channel:      "stable",
				VersionData:  pollServerVersionData("1"),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerVersionData("1"),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailReadyUpdatingOS),
		},
		{
			// The version data of a server, which has been rebooted, is outdated,
			// since the reboot activates the update, that has been applied before.
			// The maintenance state is taken from the server as well, so that a
			// server, which is still evacuated, is not reported as up to date, which
			// would make the rolling update skip its restore.
			name: "success - rebooting, server is back after the reboot",
			serverArg: provisioning.Server{
				Name:              "one",
				Cluster:           new("clusterA"),
				Status:            api.ServerStatusOffline,
				StatusDetail:      api.ServerStatusDetailOfflineRebooting,
				LastStatusUpdated: fixedDate.Add(-1 * time.Minute),
				Channel:           "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:              "one",
				Cluster:           new("clusterA"),
				Status:            api.ServerStatusOffline,
				StatusDetail:      api.ServerStatusDetailOfflineRebooting,
				LastStatusUpdated: fixedDate.Add(-1 * time.Minute),
				Channel:           "stable",
				// State from before the reboot, the update has been applied, but the
				// server has not yet been rebooted.
				VersionData: pollServerClusteredVersionData("1", "2", true, api.InMaintenanceEvacuated),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerClusteredVersionData("2", "2", false, api.InMaintenanceEvacuated),

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailNone),
			wantServerVersionData:  new(pollServerClusteredVersionData("2", "2", false, api.InMaintenanceEvacuated)),
		},
		{
			name: "success - unresponsive, server is back after the update has been applied",
			serverArg: provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusOffline,
				StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
				Channel:      "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusOffline,
				StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
				Channel:      "stable",
				VersionData:  pollServerVersionData("1"),
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: pollServerVersionData("2"),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatusDetail: new(api.ServerStatusDetailNone),
			wantServerVersionData:  new(pollServerVersionData("2")),
		},
		{
			name: "success - pending registration",
			serverArg: provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusPending,
				StatusDetail: api.ServerStatusDetailPendingRegistering,
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusPending,
				StatusDetail: api.ServerStatusDetailPendingRegistering,
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - evacuated",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyEvacuating,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:    "IncusOS",
						Version: "1",
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "1",
							InMaintenance: api.InMaintenanceEvacuated,
						},
					},
				},
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:    "IncusOS",
					Version: "1",
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:          "incus",
						Version:       "1",
						InMaintenance: api.InMaintenanceEvacuated,
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					UUID:     uuidgen.FromPattern(t, "1"),
					Version:  "1",
					Channels: []string{"stable"},
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - restoring",
			serverArg: provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyRestoring,
			},
			updateServerConfigArg: false,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Cluster:      new("cluster"),
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyRestoring,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "1",
							InMaintenance: api.NotInMaintenance,
						},
					},
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{
				UpdateStatus: api.ClusterUpdateStatus{
					InProgressStatus: api.ClusterUpdateInProgressStatus{
						InProgress: api.ClusterUpdateInProgressInactive,
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},

		{
			name: "error - client IsReady",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			clientIsReadyErr:      boom.Error,
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - server in status offline grace period",
			serverArg: provisioning.Server{
				Name:              "one",
				Status:            api.ServerStatusOffline,
				StatusDetail:      api.ServerStatusDetailOfflineRebooting,
				LastStatusUpdated: fixedDate.Add(-2 * time.Second),
			},
			updateServerConfigArg: true,

			assertErr: errassert.RetryableErrorContains("still rebooting (in reboot grace period)"),
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - client GetResources",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			clientGetResourcesErr: boom.Error,
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - client GetOSData",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			clientGetOSDataErr:    boom.Error,
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - server without ip address on management interface keeps its connection URL",
			serverArg: provisioning.Server{
				Name:          "one",
				Status:        api.ServerStatusPending,
				Channel:       "stable",
				ConnectionURL: "https://192.168.0.100:8443",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:          "one",
				Status:        api.ServerStatusPending,
				ConnectionURL: "https://192.168.0.100:8443",
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{}, // no ip present on management interface
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},

			assertErr:               require.NoError,
			assertLog:               log.Contains(`Failed to determine the connection URL of the server, keeping "https://192.168.0.100:8443"`),
			wantServerConnectionURL: new("https://192.168.0.100:8443"),
		},
		{
			name: "error - client GetVersionData",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionDataErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - update channel mismatch",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusPending,
				Channel: "stable",
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusPending,
				Channel: "stable",
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "testing", // does not match expected channel
			},

			assertErr: require.NoError,
			assertLog: log.Match(`Update channel "testing" reported by server does not match expected update channel "stable"`),
		},
		{
			name: "error - GetByName",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			repoGetByNameErr:      boom.Error,
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - pending update with server registration scriptlet error",
			serverArg: provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusPending,
				StatusDetail: api.ServerStatusDetailPendingRegistering,
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusPending,
				StatusDetail: api.ServerStatusDetailPendingRegistering,
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			runnerServerRegistrationRunErr: boom.Error,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to run server registration scriptlet: boom!"),
		},
		{
			name: "error - enrichServerWithVersionDetails",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdatingOS,
			},
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - Update",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			repoGetByName: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			updateServerConfigArg: true,
			clientGetOSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
							},
						},
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - clusterSvc.GetByName",
			serverArg: provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyRestoring,
			},
			updateServerConfigArg: false,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Cluster:      new("cluster"),
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyRestoring,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name:          "incus",
							Version:       "1",
							InMaintenance: api.NotInMaintenance,
						},
					},
				},
			},
			clusterSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					require.Equal(t, api.ServerStatusReady, server.Status)
					require.Equal(t, fixedDate, server.LastSeen)
					if tc.wantServerStatusDetail != nil {
						require.Equal(t, *tc.wantServerStatusDetail, server.StatusDetail)
					}

					if tc.wantServerVersionData != nil {
						require.Equal(t, tc.wantServerVersionData.OS, server.VersionData.OS)
						require.Equal(t, tc.wantServerVersionData.Applications, server.VersionData.Applications)
					}

					if tc.wantServerConnectionURL != nil {
						require.Equal(t, *tc.wantServerConnectionURL, server.ConnectionURL)
					}

					if tc.repoGetByName != nil && tc.repoGetByName.StatusInternal.Update != nil {
						var gotTriggeredUpdate *provisioning.ServerTriggeredUpdate
						if server.StatusInternal.Update != nil {
							gotTriggeredUpdate = server.StatusInternal.Update.Triggered
						}

						require.Equal(t, tc.wantServerTriggeredUpdate, gotTriggeredUpdate)
					}

					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientIsReadyErr
				},
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, tc.clientGetResourcesErr
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return tc.clientGetOSData, tc.clientGetOSDataErr
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return tc.clientGetVersionData, tc.clientGetVersionDataErr
				},
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
				IncusClientFunc: adapterMock.IncusClientWithoutMeshNetwork,
			}

			runner := &adapterMock.ServerScriptletPortMock{
				ServerRegistrationRunFunc: func(ctx context.Context, server *provisioning.Server) error {
					require.False(t, transaction.IsActive(ctx), "the scriptlet must run outside of a transaction")

					return tc.runnerServerRegistrationRunErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.clusterSvcGetByName, tc.clusterSvcGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.updateSvcGetAllWithFilter, tc.updateSvcGetAllWithFilterErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, runner, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithRebootStatusUpdateGracePeriod(5*time.Second),
			)

			// Run test
			err = serverSvc.PollServer(context.Background(), tc.serverArg, tc.updateServerConfigArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
		})
	}
}

func TestServerService_PollServer_resyncBMCData(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name string

		serverArg           provisioning.Server
		clientPingErr       error
		repoGetByNameServer *provisioning.Server

		bmcClientGetDataErr error

		wantBMCClientGetDataCalled bool
		assertErr                  require.ErrorAssertionFunc
		assertLog                  log.MatcherFunc
	}{
		{
			name: "success - online server without BMC configured does not resync",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
			},
			repoGetByNameServer: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
			},

			wantBMCClientGetDataCalled: false,
			assertErr:                  require.NoError,
			assertLog:                  log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - online server already reporting power state on does not resync",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
				BMCData: api.BMCData{
					ServerPowerState: "On",
				},
			},
			repoGetByNameServer: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
			},

			wantBMCClientGetDataCalled: false,
			assertErr:                  require.NoError,
			assertLog:                  log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "success - online server resyncs BMC data",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
				BMCData: api.BMCData{
					ServerPowerState: "Off",
				},
			},
			repoGetByNameServer: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
			},

			wantBMCClientGetDataCalled: true,
			assertErr:                  require.NoError,
			assertLog:                  log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - online server resyncBMCData failure is only logged",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			repoGetByNameServer: &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
			},
			bmcClientGetDataErr: boom.Error,

			wantBMCClientGetDataCalled: true,
			assertErr:                  require.NoError,
			assertLog:                  log.Match(`Failed to update BMC data for online server`),
		},
		{
			name: "success - offline server without BMC configured does not resync",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			},
			clientPingErr: boom.Error,
			repoGetByNameServer: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusOffline,
				StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
			},

			wantBMCClientGetDataCalled: false,
			assertErr:                  require.NoError,
			assertLog:                  log.Match(`Server connection test failed \(offline unresponsive\)`),
		},
		{
			name: "success - offline server already reporting power state off does not resync",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
				BMCData: api.BMCData{
					ServerPowerState: "Off",
				},
			},
			clientPingErr: boom.Error,
			repoGetByNameServer: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusOffline,
				StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
			},

			wantBMCClientGetDataCalled: false,
			assertErr:                  require.NoError,
			assertLog:                  log.Match(`Server connection test failed \(offline unresponsive\)`),
		},
		{
			name: "success - offline server resyncs BMC data",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
				BMCData: api.BMCData{
					ServerPowerState: "On",
				},
			},
			clientPingErr: boom.Error,
			repoGetByNameServer: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusOffline,
				StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
			},

			wantBMCClientGetDataCalled: true,
			assertErr:                  require.NoError,
			assertLog:                  log.Match(`Server connection test failed \(offline unresponsive\)`),
		},
		{
			name: "error - offline server resyncBMCData failure is only logged",
			serverArg: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			clientPingErr: boom.Error,
			repoGetByNameServer: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusOffline,
				StatusDetail: api.ServerStatusDetailOfflineUnresponsive,
			},
			bmcClientGetDataErr: boom.Error,

			wantBMCClientGetDataCalled: true,
			assertErr:                  require.NoError,
			assertLog:                  log.Match(`(?s)Server connection test failed \(offline unresponsive\).*Failed to update BMC data for offline server`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, nil
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			bmcClientGetDataCalled := false
			bmcClient := &adapterMock.BMCServerClientPortMock{
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					bmcClientGetDataCalled = true

					return api.BMCData{
						ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
					}, tc.bmcClientGetDataErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err = serverSvc.PollServer(context.Background(), tc.serverArg, false)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Equal(t, tc.wantBMCClientGetDataCalled, bmcClientGetDataCalled)
		})
	}
}

func TestServerService_PollServer_in_transaction(t *testing.T) {
	// Setup
	logBuf := &bytes.Buffer{}
	err := logger.InitLogger(logBuf, "", false, true, true)
	require.NoError(t, err)

	repo := &repoMock.ServerRepoMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
			return &provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusPending,
			}, nil
		},
		UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
			return nil
		},
	}

	client := &adapterMock.ServerClientPortMock{
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			return nil
		},
		IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
			return nil
		},
	}

	updateSvc := &svcMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{}, nil
		},
	}

	serverSvc := provisioningServer.New(
		repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
		provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
	)

	// Run test
	err = transaction.Do(t.Context(), func(ctx context.Context) error {
		return serverSvc.PollServer(ctx, provisioning.Server{
			Name:   "one",
			Status: api.ServerStatusPending,
		}, false)
	})

	// Assert
	require.NoError(t, err)
	log.Contains("serverService.PollServer is called inside of a DB transaction")(t, logBuf)
}

func TestServerService_ResyncByName(t *testing.T) {
	serverCertPEM, serverKeyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	serverCertificate, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                   string
		resourceTypeArg        domain.ResourceType
		lifecycleOperationArg  domain.LifecycleOperation
		repoGetByName          provisioning.Server
		repoGetByNameErr       error
		clusterSvcGetByName    *provisioning.Cluster
		clusterSvcGetByNameErr error
		repoUpdateErr          error

		assertErr    require.ErrorAssertionFunc
		wantLastSeen time.Time
	}{
		{
			name:            "success - not resource type server",
			resourceTypeArg: domain.ResourceType(""), // empty resource type

			assertErr: require.NoError,
		},
		{
			name:                  "success - update operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationUpdate,
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Status: api.ServerStatusReady,
			},

			assertErr:    require.NoError,
			wantLastSeen: fixedDate,
		},
		{
			name:                  "success - evacuate operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationEvacuate,
			repoGetByName: provisioning.Server{
				Name:   "incus",
				Type:   api.ServerTypeIncus,
				Status: api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                  "success - restore operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationRestore,
			repoGetByName: provisioning.Server{
				Name:   "incus",
				Type:   api.ServerTypeIncus,
				Status: api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                  "success - restore operation not part of cluster wide rolling update",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationRestore,
			repoGetByName: provisioning.Server{
				Name:    "incus",
				Cluster: new("cluster"),
				Type:    api.ServerTypeIncus,
				Status:  api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{
				UpdateStatus: api.ClusterUpdateStatus{
					InProgressStatus: api.ClusterUpdateInProgressStatus{
						InProgress: api.ClusterUpdateInProgressInactive,
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                  "success - evacuate operation - non incus",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationEvacuate,
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Type:   api.ServerTypeOperationsCenter, // type != incus
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:                  "success - not supported operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperation(""), // empty operation
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Type:   api.ServerTypeOperationsCenter,
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:                  "error - repo.GetByName",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationUpdate,
			repoGetByNameErr:      boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                  "error - clusterSvc.GetByName",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationRestore,
			repoGetByName: provisioning.Server{
				Name:    "incus",
				Cluster: new("cluster"),
				Type:    api.ServerTypeIncus,
				Status:  api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                  "error - pollServer",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationUpdate,
			repoGetByName: provisioning.Server{
				Name:   "incus",
				Type:   api.ServerTypeIncus,
				Status: api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr:    boom.ErrorIs,
			wantLastSeen: fixedDate,
		},
		{
			name:                  "error - evacuate operation - repo.Update",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationEvacuate,
			repoGetByName: provisioning.Server{
				Name:   "incus",
				Type:   api.ServerTypeIncus,
				Status: api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					require.Equal(t, tc.wantLastSeen, in.LastSeen)
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, nil
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{
						Network: incusosapi.SystemNetwork{
							State: incusosapi.SystemNetworkState{
								Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
									"eth0": {
										Addresses: []string{
											"192.168.0.100",
										},
										Roles: []string{
											"management",
										},
									},
								},
							},
						},
					}, nil
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return api.ServerVersionData{
						UpdateChannel: "stable",
					}, nil
				},
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.clusterSvcGetByName, tc.clusterSvcGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, serverCertificate,
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err := serverSvc.ResyncByName(t.Context(), "", domain.LifecycleEvent{
				ResourceType: tc.resourceTypeArg,
				Operation:    tc.lifecycleOperationArg,
				Source: domain.LifecycleSource{
					Name: "one",
				},
			})

			// Assert
			tc.assertErr(t, err)
		})
	}
}

// pollServerClusteredVersionData is the version data, a clustered server reports
// while it passes through an update with reboot.
func pollServerClusteredVersionData(osVersion string, osVersionNext string, needsReboot bool, inMaintenance api.InMaintenanceState) api.ServerVersionData {
	return api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "IncusOS",
			Version:     osVersion,
			VersionNext: osVersionNext,
			NeedsReboot: needsReboot,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:          "incus",
				Version:       osVersionNext,
				InMaintenance: inMaintenance,
			},
		},
		UpdateChannel: "stable",
	}
}

// pollServerApplicationVersionData is pollServerVersionData for the cases, where
// only the application has been updated, so the OS stays behind at version 1.
func pollServerApplicationVersionData(incusVersion string) api.ServerVersionData {
	return api.ServerVersionData{
		OS: api.OSVersionData{
			Name:    "IncusOS",
			Version: "1",
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:    "incus",
				Version: incusVersion,
			},
		},
		UpdateChannel: "stable",
	}
}

func pollServerVersionData(incusVersion string) api.ServerVersionData {
	return api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "IncusOS",
			Version:     "1",
			VersionNext: incusVersion,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:    "incus",
				Version: incusVersion,
			},
		},
		UpdateChannel: "stable",
	}
}
