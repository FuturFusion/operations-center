package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
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
	"github.com/FuturFusion/operations-center/shared/api/system"
)

const testSeedImageID = "a1B2c3D4e5F6"

func TestServerService_UpdateCertificate(t *testing.T) {
	config.InitTest(t, &envMock.EnvironmentMock{}, nil)

	serverCertPEM, serverKeyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	serverCertificate, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                    string
		argCertificate          tls.Certificate
		repoGetAllWithFilter    provisioning.Servers
		repoGetAllWithFilterErr error
		repoGetByName           provisioning.Server
		repoUpdateErr           error
		repoCreateErr           error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:           "success - operations center self update",
			argCertificate: serverCertificate,
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   new(string(serverCertPEM)),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
					Channel:       "stable",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                 "success - operations center self update - no server of type operations center - trigger self register",
			argCertificate:       serverCertificate,
			repoGetAllWithFilter: provisioning.Servers{},
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:                    "error - operations center self update - repo.GetAllWithFilter",
			argCertificate:          serverCertificate,
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - operations center self update - multiple servers of type operations center",
			argCertificate: serverCertificate,
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name: "one",
				},
				{
					Name: "two",
				},
			},

			assertErr: errassert.Contains(`Invalid internal state, expect at most 1 server of type "operations-center", found 2`),
		},
		// validation error not covered
		{
			name:           "error - operations center self update - repo.Update",
			argCertificate: serverCertificate,
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   new(string(serverCertPEM)),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
					Channel:       "stable",
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
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, nil
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					require.Equal(t, fixedDate, in.LastSeen)
					return tc.repoUpdateErr
				},
				CreateFunc: func(ctx context.Context, server provisioning.Server) (int64, error) {
					return 1, tc.repoCreateErr
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

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.UpdateServerCertificate(t.Context(), tc.argCertificate)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_PreRegister(t *testing.T) {
	certificatePEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)
	certificate := string(certificatePEM)

	tests := []struct {
		name          string
		server        provisioning.Server
		repoCreateErr error

		registerBMCClient    bool
		bmcConnectionTestCrt string
		bmcConnectionTestErr error

		assertErr    require.ErrorAssertionFunc
		assertServer func(t *testing.T, server provisioning.Server)
	}{
		{
			name: "success",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
			},

			assertErr:    require.NoError,
			assertServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name: "success - BMC connection test - auto pin certificate - certificate returned",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			registerBMCClient:    true,
			bmcConnectionTestCrt: "cert-pem",

			assertErr: require.NoError,
			assertServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				require.Equal(t, "cert-pem", server.BMCConfig.Certificate)
				require.False(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name: "success - BMC connection test - auto pin certificate with provided cert",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					Certificate:        certificate,
					AutoPinCertificate: true,
				},
			},
			registerBMCClient:    true,
			bmcConnectionTestCrt: "cert-pem",

			assertErr: require.NoError,
			assertServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				require.Equal(t, certificate, server.BMCConfig.Certificate)
				require.False(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name: "success - BMC connection test - not auto pin certificate - certificate not persisted",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					Certificate:        certificate,
					AutoPinCertificate: false,
				},
			},
			registerBMCClient:    true,
			bmcConnectionTestCrt: "cert-pem",

			assertErr: require.NoError,
			assertServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				require.Equal(t, certificate, server.BMCConfig.Certificate)
				require.False(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name: "error - BMC connection test - unknown BMC client type",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			registerBMCClient: false, // client for redfish-v1-generic is not registered

			assertErr:    errassert.Contains(`Failed to get BMC server client for type "redfish-v1-generic"`),
			assertServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name: "error - BMC connection test - ConnectionTest failure",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			registerBMCClient:    true,
			bmcConnectionTestErr: boom.Error,

			assertErr:    boom.ErrorIs,
			assertServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:    "", // invalid
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
			assertServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name: "error - repo.Create",
			server: provisioning.Server{
				Name:    "A",
				Status:  api.ServerStatusUnregistered,
				Channel: "stable",
			},
			repoCreateErr: boom.Error,

			assertErr:    boom.ErrorIs,
			assertServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var createdServer provisioning.Server
			repo := &repoMock.ServerRepoMock{
				CreateFunc: func(ctx context.Context, newServer provisioning.Server) (int64, error) {
					createdServer = newServer
					return -1, tc.repoCreateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ConnectionTestFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
					return tc.bmcConnectionTestCrt, tc.bmcConnectionTestErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return api.BMCData{}, errors.New("not relevant for this test")
				},
			}

			var opts []provisioningServer.Option
			if tc.registerBMCClient {
				opts = append(opts, provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient))
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{}, opts...)

			// Run test
			_, err := serverSvc.PreRegister(t.Context(), tc.server)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
			tc.assertServer(t, createdServer)
		})
	}
}

func TestServerService_Register(t *testing.T) {
	config.InitTest(t, &envMock.EnvironmentMock{}, nil)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                   string
		server                 provisioning.Server
		repoCreateErr          error
		tokenSvcConsumeErr     error
		repoGetBySystemUUID    *provisioning.Server
		repoGetBySystemUUIDErr error
		repoGetByMachineID     *provisioning.Server
		repoGetByMachineIDErr  error
		repoUpdateErr          error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - new registration",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
			},

			assertErr: require.NoError,
		},
		{
			name: "success - pre registered server by system UUID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
		`),
				SystemUUID: new("1"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:         1,
				Name:       "one",
				SystemUUID: new("1"),
			},

			assertErr: require.NoError,
		},
		{
			name: "success - pre registered server by machine ID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
		`),
				MachineID: new("1"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:        1,
				Name:      "one",
				MachineID: new("1"),
			},

			assertErr: require.NoError,
		},
		{
			name:               "error - token consume",
			tokenSvcConsumeErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - pre registered server by system UUID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
		`),
				SystemUUID: new("1"),
			},
			repoGetBySystemUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - pre registered server by machine ID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
		`),
				MachineID: new("1"),
			},
			repoGetByMachineIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - remote Operations Center",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Type: api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.ValidationErrorContains("Remote operations centers can not be registered"),
		},
		{
			name: "error - repo.Create",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.Update - pre registered server by system UUID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
		`),
				SystemUUID: new("1"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:         1,
				Name:       "one",
				SystemUUID: new("1"),
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - Ping",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
			},
			repoUpdateErr: boom.Error,

			assertErr: require.NoError, // Error of connection test is only logged, we can not assert it here.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.Server) (int64, error) {
					require.Equal(t, fixedDate, in.LastSeen)
					return 1, tc.repoCreateErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &provisioning.Server{}, nil
				},
				GetBySystemUUIDFunc: func(ctx context.Context, systemUUID string) (*provisioning.Server, error) {
					return tc.repoGetBySystemUUID, tc.repoGetBySystemUUIDErr
				},
				GetByMachineIDFunc: func(ctx context.Context, machineID string) (*provisioning.Server, error) {
					return tc.repoGetByMachineID, tc.repoGetByMachineIDErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
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
					return api.ServerVersionData{}, nil
				},
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				ConsumeFunc: func(ctx context.Context, id uuid.UUID) (string, error) {
					return "stable", tc.tokenSvcConsumeErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			token := uuid.MustParse("686d2a12-20f9-11f0-82c6-7fff26bab0c4")

			serverSvc := provisioningServer.New(
				repo, client, nil, tokenSvc, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithInitialConnectionDelay(0), // Disable delay for initial connection test
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			_, err := serverSvc.Register(t.Context(), token, tc.server)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_GetAll(t *testing.T) {
	tests := []struct {
		name              string
		repoGetAllServers provisioning.Servers
		repoGetAllErr     error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllServers: provisioning.Servers{
				provisioning.Server{
					Name:          "one",
					Cluster:       new("one"),
					ConnectionURL: "http://one/",
				},
				provisioning.Server{
					Name:          "two",
					Cluster:       new("one"),
					ConnectionURL: "http://one/",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:          "error - repo",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.repoGetAllServers, tc.repoGetAllErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			servers, err := serverSvc.GetAll(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, servers, tc.count)
		})
	}
}

func TestServerService_GetAllWithFilter(t *testing.T) {
	tests := []struct {
		name                         string
		filter                       provisioning.ServerFilter
		repoGetAllWithFilter         provisioning.Servers
		repoGetAllWithFilterErr      error
		updateSvcGetAllWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success - no filter expression",
			filter: provisioning.ServerFilter{
				Cluster: new("one"),
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
				provisioning.Server{
					Name: "two",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ServerFilter{
				Expression: new(`name == "one"`),
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
				provisioning.Server{
					Name: "two",
				},
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name:                    "error - repo",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ServerFilter{
				Expression: new(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
			},

			assertErr: errassert.ValidationErrorContains("Failed to compile filter expression:"),
			count:     0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: new(`fromBase64("~invalid") == ""`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
			},

			assertErr: errassert.ValidationErrorContains("Failed to execute filter expression:"),
			count:     0,
		},
		{
			name: "error - upodateSvc.GetAllWithFilter",
			filter: provisioning.ServerFilter{
				Cluster: new("one"),
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
				provisioning.Server{
					Name: "two",
				},
			},
			updateSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, tc.updateSvcGetAllWithFilterErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			server, err := serverSvc.GetAllWithFilter(t.Context(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, server, tc.count)
		})
	}
}

func TestServerService_GetAllNames(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllNames    []string
		repoGetAllNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllNames: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:               "error - repo",
			repoGetAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			serverNames, err := serverSvc.GetAllNames(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverNames, tc.count)
		})
	}
}

func TestServerService_GetAllNamesWithFilter(t *testing.T) {
	tests := []struct {
		name                         string
		filter                       provisioning.ServerFilter
		repoGetAllNamesWithFilter    []string
		repoGetAllNamesWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success - no filter expression",
			filter: provisioning.ServerFilter{
				Cluster: new("one"),
			},
			repoGetAllNamesWithFilter: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ServerFilter{
				Expression: new(`name matches "one"`),
			},
			repoGetAllNamesWithFilter: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ServerFilter{
				Expression: new(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: errassert.ValidationErrorContains("Failed to compile filter expression:"),
			count:     0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: new(`fromBase64("~invalid") == ""`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: errassert.ValidationErrorContains("Failed to execute filter expression:"),
			count:     0,
		},
		{
			name:                         "error - repo",
			repoGetAllNamesWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNamesWithFilter, tc.repoGetAllNamesWithFilterErr
				},
				GetAllNamesWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
					return tc.repoGetAllNamesWithFilter, tc.repoGetAllNamesWithFilterErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			serverIDs, err := serverSvc.GetAllNamesWithFilter(t.Context(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverIDs, tc.count)
		})
	}
}

func TestServerService_GetByName(t *testing.T) {
	tests := []struct {
		name                         string
		nameArg                      string
		repoGetByNameServer          *provisioning.Server
		repoGetByNameErr             error
		updateSvcGetAllWithFilter    provisioning.Updates
		updateSvcGetAllWithFilterErr error

		assertErr  require.ErrorAssertionFunc
		wantServer *provisioning.Server
	}{
		{
			name:    "success - no updates",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					NeedsUpdate:   new(false),
					NeedsReboot:   new(false),
					InMaintenance: new(api.NotInMaintenance),
					OS: api.OSVersionData{
						NeedsUpdate: new(false),
					},
				},
			},
		},
		{
			name:    "success - with version data and updates - everything up to date",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "IncusOS",
						Version:     "2",
						VersionNext: "2",
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:    "incus",
							Version: "2",
						},
						{
							Name:    "incus-ceph",
							Version: "2",
						},
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/incus-ceph.raw.gz",
						},
					},
				},
				{
					Version: "1",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/incus-ceph.raw.gz",
						},
					},
				},
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:             "IncusOS",
						Version:          "2",
						VersionNext:      "2",
						AvailableVersion: new("2"),
						NeedsUpdate:      new(false),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:             "incus",
							Version:          "2",
							AvailableVersion: new("2"),
							NeedsUpdate:      new(false),
						},
						{
							Name:             "incus-ceph",
							Version:          "2",
							AvailableVersion: new("2"),
							NeedsUpdate:      new(false),
						},
					},
					NeedsUpdate:   new(false),
					NeedsReboot:   new(false),
					InMaintenance: new(api.NotInMaintenance),
				},
			},
		},
		{
			name:    "success - with version data and updates - update available",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "IncusOS",
						Version:     "2",
						VersionNext: "2",
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:    "incus",
							Version: "2",
						},
						{
							Name:    "incus-ceph",
							Version: "2",
						},
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					Version: "3",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/incus-ceph.raw.gz",
						},
					},
				},
				{
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/incus-ceph.raw.gz",
						},
					},
				},
				{
					Version: "1",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						{
							Filename: "x86_64/incus-ceph.raw.gz",
						},
					},
				},
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:             "IncusOS",
						Version:          "2",
						VersionNext:      "2",
						AvailableVersion: new("3"),
						NeedsUpdate:      new(true),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:             "incus",
							Version:          "2",
							AvailableVersion: new("3"),
							NeedsUpdate:      new(true),
						},
						{
							Name:             "incus-ceph",
							Version:          "2",
							AvailableVersion: new("3"),
							NeedsUpdate:      new(true),
						},
					},
					NeedsUpdate:   new(true),
					NeedsReboot:   new(false),
					InMaintenance: new(api.NotInMaintenance),
				},
			},
		},
		{
			name:    "success - with version data and updates - no update information for incus-ceph",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "IncusOS",
						Version:     "2",
						VersionNext: "2",
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:    "incus",
							Version: "2",
						},
						{
							Name:    "incus-ceph",
							Version: "2",
						},
					},
				},
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						// incus-ceph missing here
					},
				},
				{
					Version: "1",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
						// incus-ceph missing here
					},
				},
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:             "IncusOS",
						Version:          "2",
						VersionNext:      "2",
						AvailableVersion: new("2"),
						NeedsUpdate:      new(false),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:             "incus",
							Version:          "2",
							AvailableVersion: new("2"),
							NeedsUpdate:      new(false),
						},
						{
							Name:        "incus-ceph",
							Version:     "2",
							NeedsUpdate: new(false),
						},
					},
					NeedsUpdate:   new(false),
					NeedsReboot:   new(false),
					InMaintenance: new(api.NotInMaintenance),
				},
			},
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - updateSvc.GetAllWithFilter",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
			},
			updateSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.updateSvcGetAllWithFilter, tc.updateSvcGetAllWithFilterErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			server, err := serverSvc.GetByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantServer, server)
		})
	}
}

func TestServerService_Update(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	certificatePEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)
	certificate := string(certificatePEM)

	tests := []struct {
		name                 string
		argForce             bool
		argBMCConnectionTest bool
		server               provisioning.Server
		repoUpdateErrs       queue.Errs
		repoGetByName        []queue.Item[*provisioning.Server]

		registerBMCClient    bool
		bmcConnectionTestCrt string
		bmcConnectionTestErr error

		wantBMCConnectionTestCalled bool

		assertErr           require.ErrorAssertionFunc
		assertLog           log.MatcherFunc
		assertUpdatedServer func(t *testing.T, server provisioning.Server)
	}{
		{
			name: "success",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
			},

			assertErr:           require.NoError,
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name: "success - BMC connection test - auto pin certificate - certificate returned",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
			},
			argBMCConnectionTest: true,
			registerBMCClient:    true,
			bmcConnectionTestCrt: "cert-pem",

			wantBMCConnectionTestCalled: true,

			assertErr: require.NoError,
			assertLog: log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				require.Equal(t, "cert-pem", server.BMCConfig.Certificate)
				require.False(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name: "success - BMC connection test - not auto pin certificate - certificate not persisted",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					Certificate:        certificate,
					AutoPinCertificate: false,
				},
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
			},
			argBMCConnectionTest: true,
			registerBMCClient:    true,
			bmcConnectionTestCrt: "cert-pem",

			wantBMCConnectionTestCalled: true,

			assertErr: require.NoError,
			assertLog: log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				require.Equal(t, certificate, server.BMCConfig.Certificate)
				require.False(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name: "success - BMC connection test - auto pin certificate with provided certificate",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					Certificate:        certificate,
					AutoPinCertificate: true,
				},
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
			},
			argBMCConnectionTest: true,
			registerBMCClient:    true,
			bmcConnectionTestCrt: "cert-pem",

			wantBMCConnectionTestCalled: true,

			assertErr: require.NoError,
			assertLog: log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				require.Equal(t, certificate, server.BMCConfig.Certificate)
				require.False(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name:     "error - BMC connection test - unknown BMC client type",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			argBMCConnectionTest: true,
			registerBMCClient:    false, // client for redfish-v1-generic is not registered

			assertErr:           errassert.Contains(`Failed to get BMC server client for type "redfish-v1-generic"`),
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name:     "error - BMC connection test - ConnectionTest failure",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			argBMCConnectionTest: true,
			registerBMCClient:    true,
			bmcConnectionTestErr: boom.Error,

			wantBMCConnectionTestCalled: true,

			assertErr:           boom.ErrorIs,
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name:     "success - BMC connection test not requested - unreachable BMC does not fail the server state update",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
			},
			argBMCConnectionTest: false,
			registerBMCClient:    true,
			bmcConnectionTestErr: boom.Error,

			wantBMCConnectionTestCalled: false,

			assertErr: require.NoError,
			assertLog: log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) {
				t.Helper()
				// The BMC config is persisted unmodified, pinning of the certificate
				// is deferred to the next update, which requests a connection test.
				require.Empty(t, server.BMCConfig.Certificate)
				require.True(t, server.BMCConfig.AutoPinCertificate)
			},
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr:           errassert.ValidationError,
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name:     "error - repo.GetByName - without force",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr:           boom.ErrorIs,
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name:     "error - channel update for clustered server",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("one"),
						Channel: "testing",
					},
				},
			},

			assertErr:           errassert.OperationNotPermittedErrorContains(`Update of channel not allowed for clustered server "one"`),
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name: "error - repo.UpdateByID",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
			},
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr:           boom.ErrorIs,
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name:     "error - repo.GetByName - force", // UpdateSystemUpdate
			argForce: true,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Err: boom.Error,
				},
			},

			assertErr:           boom.ErrorIs,
			assertLog:           log.Empty,
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
		{
			name:     "error - repo.GetByName - force - revert error", // UpdateSystemUpdate
			argForce: true,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
					},
				},
				{
					Err: boom.Error,
				},
			},
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},

			assertErr:           boom.ErrorIs,
			assertLog:           log.Contains("Failed to restore previous server state after failed to update system update config"),
			assertUpdatedServer: func(t *testing.T, server provisioning.Server) { t.Helper() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			var updatedServer provisioning.Server
			repo := &repoMock.ServerRepoMock{
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					updatedServer = in

					return tc.repoUpdateErrs.PopOrNil(t)
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
			}

			bmcClientConnectionTestCalled := false
			bmcClient := &adapterMock.BMCServerClientPortMock{
				ConnectionTestFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
					bmcClientConnectionTestCalled = true

					return tc.bmcConnectionTestCrt, tc.bmcConnectionTestErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return errors.New("") // short circuit pollServer, since we don't care about this part in this test.
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
				UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
					return nil
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return &provisioning.Channel{}, nil
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			opts := []provisioningServer.Option{
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			}
			if tc.registerBMCClient {
				opts = append(opts, provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient))
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, channelSvc, updateSvc, tls.Certificate{},
				opts...,
			)

			// Run test
			err = serverSvc.Update(t.Context(), tc.server, tc.argForce, true, tc.argBMCConnectionTest)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			tc.assertUpdatedServer(t, updatedServer)

			require.Equal(t, tc.wantBMCConnectionTestCalled, bmcClientConnectionTestCalled)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_UpdateSystemNetwork(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	type repoUpdateFuncItem struct {
		lastSeen time.Time
		status   api.ServerStatus
	}

	tests := []struct {
		name                         string
		ctx                          context.Context
		repoGetByNameServer          provisioning.Server
		repoGetByNameErr             error
		repoUpdate                   []queue.Item[repoUpdateFuncItem]
		clientUpdateNetworkConfigErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			ctx:              t.Context(),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.UpdateByID",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateNetworkConfig with cancelled context with cause",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancelCause(t.Context())
				cancel(nil)
				return ctx
			}(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, context.Canceled)
			},
		},
		{
			name: "error - client.UpdateNetworkConfig",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},
			clientUpdateNetworkConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateNetworkConfig - reverter error",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
					Err: errors.New("reverter"),
				},
			},
			clientUpdateNetworkConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					value, err := queue.Pop(t, &tc.repoUpdate)

					require.Equal(t, value.lastSeen, in.LastSeen)
					require.Equal(t, value.status, in.Status)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateNetworkConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					return errors.Join(tc.clientUpdateNetworkConfigErr, ctx.Err())
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			// Register our own self update signal, such that we can ensure, that all the listeners
			// have been removed after successful processing.
			selfUpdateSignal := signals.New[provisioning.Server]()

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithSelfUpdateSignal(selfUpdateSignal),
			)

			// Run test
			err := serverSvc.UpdateSystemNetwork(tc.ctx, "one", provisioning.ServerSystemNetwork{})

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
			require.True(t, selfUpdateSignal.IsEmpty())
		})
	}
}

func TestServerService_UpdateSystemStorage(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	type repoUpdateFuncItem struct {
		lastSeen time.Time
		status   api.ServerStatus
	}

	tests := []struct {
		name                         string
		ctx                          context.Context
		repoGetByNameServer          provisioning.Server
		repoGetByNameErr             error
		repoUpdate                   []queue.Item[repoUpdateFuncItem]
		clientUpdateStorageConfigErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			ctx:              t.Context(),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.UpdateByID",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateStorageConfig with cancelled context with cause",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancelCause(t.Context())
				cancel(nil)
				return ctx
			}(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, context.Canceled)
			},
		},
		{
			name: "error - client.UpdateStorageConfig",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},
			clientUpdateStorageConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateStorageConfig - reverter error",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
					Err: errors.New("reverter"),
				},
			},
			clientUpdateStorageConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					value, err := queue.Pop(t, &tc.repoUpdate)

					require.Equal(t, value.lastSeen, in.LastSeen)
					require.Equal(t, value.status, in.Status)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateStorageConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					return errors.Join(tc.clientUpdateStorageConfigErr, ctx.Err())
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
			)

			// Run test
			err := serverSvc.UpdateSystemStorage(tc.ctx, "one", provisioning.ServerSystemStorage{})

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
		})
	}
}

func TestServerService_GetSystemProvider(t *testing.T) {
	tests := []struct {
		name                       string
		repoGetByNameServer        provisioning.Server
		repoGetByNameErr           error
		clientGetProviderConfig    provisioning.ServerSystemProvider
		clientGetProviderConfigErr error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemProvider
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetProviderConfig: provisioning.ServerSystemProvider{
				Config: incusosapi.SystemProviderConfig{
					Name: "operations-center",
				},
				State: incusosapi.SystemProviderState{
					Registered: true,
				},
			},

			assertErr: require.NoError,
			want: provisioning.ServerSystemProvider{
				Config: incusosapi.SystemProviderConfig{
					Name: "operations-center",
				},
				State: incusosapi.SystemProviderState{
					Registered: true,
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetProviderConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetProviderConfigFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemProvider, error) {
					return tc.clientGetProviderConfig, tc.clientGetProviderConfigErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			got, err := serverSvc.GetSystemProvider(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_UpdateSystemProvider(t *testing.T) {
	tests := []struct {
		name                          string
		repoGetByNameServer           provisioning.Server
		repoGetByNameErr              error
		clientUpdateProviderConfigErr error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemProvider
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateProviderConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status: api.ServerStatusReady,
			},
			clientUpdateProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateProviderConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemProvider) error {
					return tc.clientUpdateProviderConfigErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemProvider(
				t.Context(), "one", incusosapi.SystemProvider{
					Config: incusosapi.SystemProviderConfig{
						Name: "operations-center-new",
					},
				},
			)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_GetSystemUpdate(t *testing.T) {
	tests := []struct {
		name                     string
		repoGetByNameServer      provisioning.Server
		repoGetByNameErr         error
		clientGetUpdateConfig    provisioning.ServerSystemUpdate
		clientGetUpdateConfigErr error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemUpdate
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfig: provisioning.ServerSystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},

			assertErr: require.NoError,
			want: provisioning.ServerSystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetUpdateConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetUpdateConfigFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemUpdate, error) {
					return tc.clientGetUpdateConfig, tc.clientGetUpdateConfigErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			got, err := serverSvc.GetSystemUpdate(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_UpdateSystemUpdate(t *testing.T) {
	tests := []struct {
		name                        string
		repoGetByNameServer         provisioning.Server
		repoGetByNameErr            error
		clientGetUpdateConfig       provisioning.ServerSystemUpdate
		clientUpdateUpdateConfigErr error
		channelSvcGetByNameErr      error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemUpdate
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfig: incusosapi.SystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                   "error - updateSvc.GetChannelByName",
			channelSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateUpdateConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfig: incusosapi.SystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},
			clientUpdateUpdateConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
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
					return api.HardwareData{}, boom.Error // Since we do not care too much, if the server poll was successful, we always return an error here.
				},
				UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, updateConfig provisioning.ServerSystemUpdate) error {
					require.False(t, updateConfig.Config.AutoReboot)              // AutoReboot is forced to false.
					require.Equal(t, "never", updateConfig.Config.CheckFrequency) // CheckFrequency is forced to "never".
					return tc.clientUpdateUpdateConfigErr
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return &provisioning.Channel{}, tc.channelSvcGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, channelSvc, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemUpdate(
				t.Context(), "one", incusosapi.SystemUpdate{
					Config: incusosapi.SystemUpdateConfig{
						AutoReboot:     true,
						Channel:        "testing",
						CheckFrequency: "2h",
					},
				},
			)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_UpdateSystemNetworkWithSelfUpdateSignal(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	type repoUpdateFuncItem struct {
		lastSeen time.Time
		status   api.ServerStatus
	}

	tests := []struct {
		name                         string
		repoGetByNameServer          provisioning.Server
		repoGetByNameErr             error
		repoUpdate                   []queue.Item[repoUpdateFuncItem]
		clientUpdateNetworkConfigErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
			},

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					value, err := queue.Pop(t, &tc.repoUpdate)

					require.Equal(t, value.lastSeen, in.LastSeen)
					require.Equal(t, value.status, in.Status)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateNetworkConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					// Simulate network change, which prevents a clean response.
					<-ctx.Done()
					return ctx.Err()
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			selfUpdateSignal := signals.New[provisioning.Server]()

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithSelfUpdateSignal(selfUpdateSignal),
			)

			// Run test
			wg := sync.WaitGroup{}
			wg.Add(1)

			var err error
			go func() {
				defer wg.Done()

				err = serverSvc.UpdateSystemNetwork(t.Context(), "one", provisioning.ServerSystemNetwork{})
			}()

			// Wait for subscriber.
			for selfUpdateSignal.IsEmpty() {
				time.Sleep(time.Millisecond)
			}

			// Simulate update from a different node, which is ignored.
			selfUpdateSignal.Emit(t.Context(), provisioning.Server{
				Name: "another",
			})

			selfUpdateSignal.Emit(t.Context(), provisioning.Server{
				Name: "one",
			})

			wg.Wait()

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
			require.True(t, selfUpdateSignal.IsEmpty())
		})
	}
}

func TestServerService_SelfUpdate(t *testing.T) {
	serverCertPEM, serverKeyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	serverCertificate, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                       string
		serverSelfUpdate           provisioning.ServerSelfUpdate
		repoGetAllWithFilter       provisioning.Servers
		repoGetAllWithFilterErr    error
		repoGetByCertificateServer *provisioning.Server
		repoGetByCertificateErr    error
		repoUpdateErr              error
		repoGetByNameErr           error

		assertErr              require.ErrorAssertionFunc
		assertLog              log.MatcherFunc
		wantServerStatus       api.ServerStatus
		wantServerStatusDetail api.ServerStatusDetail
	}{
		{
			name: "success",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause network config changed",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseNetworkConfigChanged,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause system is ready for system in pending registering state",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseSystemIsReady,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusPending,
				StatusDetail:  api.ServerStatusDetailPendingRegistering,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusPending,
			wantServerStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name: "success - cause system is ready",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseSystemIsReady,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause OS update applied keeps updating state",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseOSUpdateApplied,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				StatusDetail:  api.ServerStatusDetailReadyUpdatingOS,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailReadyUpdatingOS,
		},
		{
			name: "success - cause application update applied",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseApplicationUpdateApplied,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				StatusDetail:  api.ServerStatusDetailReadyUpdatingApplication,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause network interface state changed",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseNetworkInterfaceStateChanged,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause storage config changed",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseStorageConfigChanged,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause system reboot triggered",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseSystemRebootTriggered,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusOffline,
			wantServerStatusDetail: api.ServerStatusDetailOfflineRebooting,
		},
		{
			name: "success - cause shutdown triggered",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseShutdownTriggered,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusOffline,
			wantServerStatusDetail: api.ServerStatusDetailOfflineShutdown,
		},
		{
			name: "success - rebooting",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusOffline,
				StatusDetail:  api.ServerStatusDetailOfflineRebooting,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusOffline,
			wantServerStatusDetail: api.ServerStatusDetailOfflineRebooting,
		},
		{
			name: "success - operations center self update",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				Self: true,
			},
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   new(string(serverCertPEM)),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
					Channel:       "stable",
				},
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause secure boot update applied",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseSecureBootUpdateApplied,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - cause suspend triggered",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseSuspendTriggered,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr:              require.NoError,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "success - operations center self update - other cause",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				Self:  true,
				Cause: api.ServerSelfUpdateCause("other-cause"),
			},
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   new(string(serverCertPEM)),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
					Channel:       "stable",
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - repo.GetByCertificate not found",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateErr: domain.ErrNotFound,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthorized)
			},
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - repo.GetByCertificate",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - undefined update cause",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCause("other-cause"),
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Ignoring unknown server self update cause server_self_update_cause=other-cause"),
		},
		{
			name: "error - validation",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             ":|//", // invalid URL
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr: errassert.ValidationError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
		},
		{
			name: "error - repo.UpdateByID",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},
			repoUpdateErr: boom.Error,

			assertErr:              boom.ErrorIs,
			assertLog:              log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
		{
			name: "error - repo.GetByName",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(string(serverCertPEM)),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},
			repoGetByNameErr: boom.Error,

			assertErr:              require.NoError, // handled async in Goroutine, error is logged.
			assertLog:              log.Contains("Failed to update server configuration after self update"),
			wantServerStatus:       api.ServerStatusReady,
			wantServerStatusDetail: api.ServerStatusDetailNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			var gotServerStatus api.ServerStatus
			var gotServerStatusDetail api.ServerStatusDetail
			var gotServerUpdated bool

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &provisioning.Server{}, tc.repoGetByNameErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetByCertificateFunc: func(ctx context.Context, certificatePEM string) (*provisioning.Server, error) {
					return tc.repoGetByCertificateServer, tc.repoGetByCertificateErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					// Only record the update performed by the self update itself. The
					// background poll triggered by it performs further updates, which
					// are not subject of this test.
					if !gotServerUpdated {
						gotServerUpdated = true
						gotServerStatus = in.Status
						gotServerStatusDetail = in.StatusDetail
					}

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

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithInitialConnectionDelay(1*time.Millisecond),
			)

			// Run test
			err = serverSvc.SelfUpdate(t.Context(), tc.serverSelfUpdate)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Equal(t, tc.wantServerStatus, gotServerStatus)
			require.Equal(t, tc.wantServerStatusDetail, gotServerStatusDetail)
		})
	}
}

func TestServerService_SelfRegisterOperationsCenter(t *testing.T) {
	serverCertPEM, serverKeyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	serverCertificate, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                    string
		repoGetAllWithFilter    provisioning.Servers
		repoGetAllWithFilterErr error
		repoCreateID            int64
		repoCreateErr           error
		repoGetByName           provisioning.Server
		clientGetResourcesErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                 "success - Operations Center initial self update (registration)",
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateID:         1,
			repoGetByName: provisioning.Server{
				Name:    "operations-center",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:                    "error - repo.GetAllWithFilter",
			repoGetAllWithFilterErr: boom.Error,
			repoCreateID:            1,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - Operations Center is already registered",
			repoGetAllWithFilter: provisioning.Servers{
				{},
			},
			repoCreateID: 1,

			assertErr: require.Error,
		},
		{
			name:                 "error - repo.Create",
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateErr:        boom.Error,
			repoCreateID:         1,

			assertErr: boom.ErrorIs,
		},
		{
			name:                 "error - client.GetResources",
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateID:         1,
			repoGetByName: provisioning.Server{
				Name:    "operations-center",
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			clientGetResourcesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return true
				},
			}, nil)

			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByName, nil
				},
				CreateFunc: func(ctx context.Context, server provisioning.Server) (int64, error) {
					return tc.repoCreateID, tc.repoCreateErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					require.Equal(t, api.ServerStatusReady, server.Status)
					require.Equal(t, fixedDate, server.LastSeen)
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
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, tc.clientGetResourcesErr
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

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			err = config.UpdateNetwork(t.Context(), system.NetworkPut{
				OperationsCenterAddress: "https://192.168.1.200:8443",
				RestServerAddress:       "[::]:8443",
			})
			require.NoError(t, err)

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.SelfRegisterOperationsCenter(t.Context())

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_Rename(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                string
		oldName             string
		newName             string
		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error
		repoRenameErr       error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			oldName: "one",
			newName: "one-new",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			oldName: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - new name empty",
			oldName: "one",
			newName: "", // invalid

			assertErr: errassert.ValidationError,
		},
		{
			name:    "error - old and new name equal",
			oldName: "one",
			newName: "one", // equal

			assertErr: errassert.ValidationError,
		},
		{
			name:             "error - repo.GetByName",
			oldName:          "one",
			newName:          "two",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - server is clustered",
			oldName: "one",
			newName: "two",
			repoGetByNameServer: &provisioning.Server{
				Name:    "one",
				Cluster: new("one"), // server already clustered
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - repo.Rename",
			oldName: "one",
			newName: "one-new",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			repoRenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				RenameFunc: func(ctx context.Context, oldName string, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.Rename(t.Context(), tc.oldName, tc.newName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error
		repoDeleteByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Cluster: nil,
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - assigned to cluster",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Cluster: new("one"),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to delete server, server is part of cluster "one"`)
			},
		},
		{
			name:    "error - repo.DeleteByName",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Cluster: nil,
			},
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			err := serverSvc.DeleteByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

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

		assertErr              require.ErrorAssertionFunc
		assertLog              log.MatcherFunc
		wantServerStatusDetail *api.ServerStatusDetail
		wantServerVersionData  *api.ServerVersionData
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
			name: "error - server without ip address on management interface",
			serverArg: provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusPending,
				Channel: "stable",
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
				UpdateChannel: "testing", // does not match expected channel
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to determine an IP address for the network interface with "management" role`)
			},
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
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
			}

			runner := &adapterMock.ServerScriptletPortMock{
				ServerRegistrationRunFunc: func(ctx context.Context, server *provisioning.Server) error {
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

func TestServerService_GetChangelogByName(t *testing.T) {
	updateV1UUID := uuidgen.FromPattern(t, "1")
	updateV2UUID := uuidgen.FromPattern(t, "2")

	tests := []struct {
		name                      string
		nameArg                   string
		repoGetByName             []queue.Item[*provisioning.Server]
		updateSvcGetAllWithFilter []queue.Item[provisioning.Updates]
		updateSvcGetChangelog     api.UpdateChangelog
		updateSvcGetChangelogErr  error

		assertErr     require.ErrorAssertionFunc
		wantChangelog api.UpdateChangelog
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
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
						{
							UUID:     updateV1UUID,
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
				},
				// updateSvc.GetAllWithFilter
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
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
						{
							UUID:     updateV1UUID,
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
				},
			},
			updateSvcGetChangelog: api.UpdateChangelog{
				CurrentVersion: "2",
				PriorVersion:   "1",
				Components: map[string]images.ChangelogEntries{
					"IncusOS": {
						Updated: []string{"file version 1 to version 2"},
					},
					"incus": {
						Updated: []string{"file version 1 to version 2"},
					},
				},
			},

			assertErr: require.NoError,
			wantChangelog: images.Changelog{
				CurrentVersion: "2",
				PriorVersion:   "1",
				Channel:        "stable",
				Components: map[string]images.ChangelogEntries{
					"IncusOS": {
						Updated: []string{"file version 1 to version 2"},
					},
					"incus": {
						Updated: []string{"file version 1 to version 2"},
					},
				},
			},
		},
		{
			name:    "success - no update available",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						// No update available.
						{
							UUID:     updateV1UUID,
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
				},
			},

			assertErr: require.NoError,
		},

		{
			name:    "error - GetByName",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - updateSvc.GetAllWithFitler",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
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
						{
							UUID:     updateV1UUID,
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
				},
				// updateSvc.GetAllWithFilter
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - updateSvc.GetChangelog",
			nameArg: "one",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Channel: "stable",
						VersionData: api.ServerVersionData{
							OS: api.OSVersionData{
								Name:    "IncusOS",
								Version: "1",
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "1",
								},
							},
						},
					},
				},
			},
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				// GetByName
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
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
						{
							UUID:     updateV1UUID,
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
				},
				// updateSvc.GetAllWithFilter
				{
					Value: provisioning.Updates{
						{
							UUID:     updateV2UUID,
							Version:  "2",
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
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Filename: "x86_64/IncusOS_202606100326.img.gz",
								},
								{
									Filename: "x86_64/incus.raw.gz",
								},
							},
						},
					},
				},
			},
			updateSvcGetChangelogErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return queue.Pop(t, &tc.updateSvcGetAllWithFilter)
				},
				GetChangelogFunc: func(ctx context.Context, currentID, priorID uuid.UUID, architecture images.UpdateFileArchitecture) (api.UpdateChangelog, error) {
					require.Equal(t, updateV2UUID, currentID)
					require.Equal(t, updateV1UUID, priorID)
					return tc.updateSvcGetChangelog, tc.updateSvcGetChangelogErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			changelog, err := serverSvc.GetChangelogByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantChangelog, changelog)
			require.Empty(t, tc.repoGetByName)
		})
	}
}

func TestServerService_EvacuateSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argClusterUpdate                                bool
		argForce                                        bool
		repoGetByName                                   []queue.Item[*provisioning.Server]
		repoUpdateErrs                                  queue.Errs
		clientEvacuateErr                               error
		clusterSvcGetByName                             *provisioning.Cluster
		clusterSvcGetByNameErr                          error
		clusterSvcUpdateErr                             error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		doCallback                                      func(f func(ctx context.Context, err error))
		initVolatileServerState                         func(serverSvc provisioning.ServerService)

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - operation in flight",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(_ func(ctx context.Context, err error)) {
				// don't perform the callback
			},
			initVolatileServerState: func(serverSvc provisioning.ServerService) {
				_ = serverSvc.EvacuateSystemByName(context.Background(), "one", true, false)
			},

			assertErr: errassert.RetryableErrorContains("server operation in flight"),
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - attempt limit reached",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clientEvacuateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},
			initVolatileServerState: func(serverSvc provisioning.ServerService) {
				_ = serverSvc.EvacuateSystemByName(context.Background(), "one", true, false)
				_ = serverSvc.EvacuateSystemByName(context.Background(), "one", true, false)
				_ = serverSvc.EvacuateSystemByName(context.Background(), "one", true, false)
			},

			assertErr: errassert.TerminalErrorContains("Failed to evacuate system in 3 attempts"),
			assertLog: log.Noop,
		},
		{
			name: "error - callback error",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to evacuate system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error - status update successful",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to evacuate system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error - GetByName",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Err: boom.Error,
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to restore DB state during rolling update on evacuation error err="Failed to get server \"one\" by name:`)(t, logBuf)
			},
		},
		{
			name:             "error - cluster update - callback error - Cluster nil",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: nil, // cluster nil
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to restore DB state during rolling update on evacuation error err="Server \"one\" is not part of a cluster`)(t, logBuf)
			},
		},
		{
			name:             "error - cluster update - callback error - clusterSvc.GetByName",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcGetByNameErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to restore DB state during rolling update on evacuation error err="Failed to get cluster \"cluster\":`)(t, logBuf)
			},
		},
		{
			name:             "error - cluster update - callback error - clusterSvc.Update",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{},
			clusterSvcUpdateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to restore DB state during rolling update on evacuation error err="Failed to update cluster \"cluster\":`)(t, logBuf)
			},
		},
		{
			name:             "error - cluster update - callback error - repo.Update",
			argClusterUpdate: true,
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clusterSvcGetByName: &provisioning.Cluster{},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: func(t log.TestifyT, logBuf *bytes.Buffer) {
				log.Contains(`Failed to evacuate system name=one err=boom!`)(t, logBuf)
				log.Contains(`Failed to restore DB state during rolling update on evacuation error err="Failed to put server \"one\" back in ready state:`)(t, logBuf)
			},
		},
		{
			name: "error - repo.GetByName",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeOperationsCenter,
					},
				},
			},
			name: "error - not type incus",

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Evacuate",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientEvacuateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Evacuate - reverter error",
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name:    "one",
						Cluster: new("cluster"),
						Status:  api.ServerStatusReady,
						Type:    api.ServerTypeIncus,
						VersionData: api.ServerVersionData{
							Applications: []api.ApplicationVersionData{
								{
									Name: "incus",
								},
							},
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientEvacuateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Contains("Failed to restore previous server state after failed to trigger evacuation server=one err=boom!"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// if tc.name != "error - cluster update - callback error" {
			// 	t.SkipNow()
			// }
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				EvacuateFunc: func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
					tc.doCallback(callback)
					return tc.clientEvacuateErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.clusterSvcGetByName, tc.clusterSvcGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster, updateServers bool) error {
					return tc.clusterSvcUpdateErr
				},
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			if tc.initVolatileServerState != nil {
				tc.initVolatileServerState(serverSvc)
			}

			// Run test
			err = serverSvc.EvacuateSystemByName(t.Context(), "one", tc.argClusterUpdate, tc.argForce)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.repoGetByName)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_PoweroffSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argForce                                        bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientPoweroffErr                               error
		clusterSvcIsInstanceLifecycleOperationPermitted bool

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Poweroff",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientPoweroffErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Poweroff and reverter error",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientPoweroffErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to trigger poweroff server=one err=boom!"),
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
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PoweroffFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientPoweroffErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.PoweroffSystemByName(t.Context(), "one", tc.argForce)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_RebootSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argForce                                        bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientRebootErr                                 error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		initVolatileServerState                         func(serverSvc provisioning.ServerService)

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - operation in flight",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			initVolatileServerState: func(serverSvc provisioning.ServerService) {
				_ = serverSvc.RebootSystemByName(context.Background(), "one", true)
			},

			assertErr: errassert.RetryableErrorContains("server operation in flight"),
			assertLog: log.Noop,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Reboot",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientRebootErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Reboot and reverter error",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientRebootErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to trigger reboot server=one err=boom!"),
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
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				RebootFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientRebootErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			if tc.initVolatileServerState != nil {
				tc.initVolatileServerState(serverSvc)
			}

			// Run test
			err = serverSvc.RebootSystemByName(t.Context(), "one", tc.argForce)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_RestoreSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argClusterUpdate                                bool
		argForce                                        bool
		argRestoreModeSkip                              bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientRestoreErr                                error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		doCallback                                      func(f func(ctx context.Context, err error))
		initVolatileServerState                         func(serverSvc provisioning.ServerService)

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - lifecycle operation permitted",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - force",
			argForce: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - operation in flight",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(_ func(ctx context.Context, err error)) {
				// don't perform the callback
			},
			initVolatileServerState: func(serverSvc provisioning.ServerService) {
				_ = serverSvc.RestoreSystemByName(context.Background(), "one", true, false, false)
			},

			assertErr: errassert.RetryableErrorContains("server operation in flight"),
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - attempt limit reached",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clientRestoreErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},
			initVolatileServerState: func(serverSvc provisioning.ServerService) {
				_ = serverSvc.RestoreSystemByName(context.Background(), "one", true, false, false)
				_ = serverSvc.RestoreSystemByName(context.Background(), "one", true, false, false)
				_ = serverSvc.RestoreSystemByName(context.Background(), "one", true, false, false)
			},

			assertErr: errassert.TerminalErrorContains("Failed to restore system in 3 attempts"),
			assertLog: log.Noop,
		},
		{
			name: "error - callback error",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to restore system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error",
			argClusterUpdate: true,
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to restore system name=one err=boom!"),
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - not type incus",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Restore",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientRestoreErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.Restore and reverter error",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "incus",
						},
					},
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientRestoreErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to trigger restore server=one err=boom!"),
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
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				RestoreFunc: func(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
					tc.doCallback(callback)
					return tc.clientRestoreErr
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			if tc.initVolatileServerState != nil {
				tc.initVolatileServerState(serverSvc)
			}

			// Run test
			err = serverSvc.RestoreSystemByName(t.Context(), "one", tc.argClusterUpdate, tc.argForce, tc.argRestoreModeSkip)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_PostRestoreSystemDoneByName(t *testing.T) {
	tests := []struct {
		name               string
		argRestoreModeSkip bool
		repoGetByName      provisioning.Server
		repoGetByNameErr   error
		repoUpdateErr      error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
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
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - not type incus",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusReady,
				Type:   api.ServerTypeIncus,
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
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err := serverSvc.PostRestoreSystemDoneByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_UpdateSystemByName(t *testing.T) {
	tests := []struct {
		name                                            string
		argUpdateRequest                                api.ServerUpdatePost
		argForce                                        bool
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientUpdateOSErr                               error
		channelSvcGetByNameErr                          error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		registerUnreachableBMCClient                    bool

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name: "success - no update triggered",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name: "success - trigger OS update - lifecycle operation permitted",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:     "success - trigger OS update - force",
			argForce: true,
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - server not ready",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusPending, // server not ready
			},

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name: "error - cluster lifecycle operation not permitted",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name: "error - repo.Update",
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - UpdateSystemUpdate - channelSvc.GetByName",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			channelSvcGetByNameErr:                          boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.UpdateOS",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientUpdateOSErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name: "error - client.UpdateOS and reverter error",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientUpdateOSErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to update the system server=one err=.*boom!"),
		},
		{
			name: "success - trigger OS update - unreachable BMC",
			argUpdateRequest: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:          "operations-center",
				Type:          api.ServerTypeOperationsCenter,
				Channel:       "stable",
				ConnectionURL: "https://one/",
				Certificate:   new("certificate"),
				Status:        api.ServerStatusReady,
				BMCConfig: api.BMCConfig{
					APIType:            api.BMCAPITypeRedfishV1Generic,
					Endpoint:           "https://bmc.example.com/",
					AutoPinCertificate: true,
				},
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			registerUnreachableBMCClient:                    true,

			assertErr: require.NoError,
			assertLog: log.Noop,
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
					// Return a copy, since the server is also queried by the background
					// poll, which would otherwise share the same instance.
					server := tc.repoGetByName
					return &server, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					// The background poll marks the server as unresponsive, since the
					// connection test is mocked to fail. Such updates are not subject of
					// this test and must not consume from the queue.
					if server.StatusDetail == api.ServerStatusDetailOfflineUnresponsive {
						return nil
					}

					return tc.repoUpdateErrs.PopOrNil(t)
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateOSFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientUpdateOSErr
				},
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return errors.New("") // short circuit pollServer, since we don't care about this part in this test.
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
				UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
					return nil
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				IsInstanceLifecycleOperationPermittedFunc: func(ctx context.Context, name string) bool {
					return tc.clusterSvcIsInstanceLifecycleOperationPermitted
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return &provisioning.Channel{}, tc.channelSvcGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			opts := []provisioningServer.Option{
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			}
			if tc.registerUnreachableBMCClient {
				bmcClient := &adapterMock.BMCServerClientPortMock{
					ConnectionTestFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
						return "", boom.Error
					},
				}

				opts = append(opts, provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient))
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, clusterSvc, channelSvc, updateSvc, tls.Certificate{},
				opts...,
			)

			// Run test
			err = serverSvc.UpdateSystemByName(t.Context(), "one", tc.argUpdateRequest, tc.argForce)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.repoUpdateErrs)
		})
	}
}

func TestServerService_FactoryResetByName(t *testing.T) {
	tests := []struct {
		name                              string
		argName                           string
		argTokenID                        *uuid.UUID
		argTokenSeedName                  *string
		repoGetByName                     provisioning.Server
		repoGetByNameErr                  error
		clientPingErr                     error
		clientSystemFactoryResetErr       error
		tokenSvcGetTokenSeedByName        *provisioning.TokenSeed
		tokenSvcGetTokenSeedByNameErr     error
		tokenSvcCreate                    provisioning.Token
		tokenSvcCreateErr                 error
		tokenSvcGetTokenProviderConfig    *api.TokenProviderConfig
		tokenSvcGetTokenProviderConfigErr error
		repoDeleteByNameErr               error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success - without tokenID and without tokenSeedName",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					Applications: []api.ApplicationVersionData{
						{
							Name: "debug",
						},
						{
							Name: "incus-lts-7.0",
						},
					},
				},
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},

			assertErr: require.NoError,
		},
		{
			name:             "success - with tokenID and tokenSeedName",
			argName:          "one",
			argTokenID:       new(uuidgen.FromPattern(t, "1")),
			argTokenSeedName: new("some_seed"),
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenSeedByName:     &provisioning.TokenSeed{},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			argName: "",

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - repo.GetByName",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - operations center",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "operations-center",
				Type: api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - incus clustered",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name:    "server01",
				Cluster: new("cluster"),
				Type:    api.ServerTypeIncus,
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - client.Ping",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - tokenSvc.GetTokenSeedByName",
			argName:          "one",
			argTokenID:       new(uuidgen.FromPattern(t, "1")),
			argTokenSeedName: new("some_seed"),
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenSeedByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - tokenSvc.Create",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			tokenSvcCreateErr:              boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - tokenSvc.GetTokenProviderConfig",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.SystemFactoryReset",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			clientSystemFactoryResetErr:    boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.DeleteByName",
			argName: "one",
			repoGetByName: provisioning.Server{
				Name: "server01",
				Type: api.ServerTypeIncus,
			},
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			repoDeleteByNameErr:            boom.Error,

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
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				SystemFactoryResetFunc: func(ctx context.Context, endpoint provisioning.Endpoint, allowTPMResetFailure bool, seeds provisioning.TokenImageSeedConfigs, providerConfig api.TokenProviderConfig) error {
					return tc.clientSystemFactoryResetErr
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return tc.tokenSvcGetTokenSeedByName, tc.tokenSvcGetTokenSeedByNameErr
				},
				CreateFunc: func(ctx context.Context, token provisioning.Token) (provisioning.Token, error) {
					return tc.tokenSvcCreate, tc.tokenSvcCreateErr
				},
				GetTokenProviderConfigFunc: func(ctx context.Context, id uuid.UUID) (*api.TokenProviderConfig, error) {
					return tc.tokenSvcGetTokenProviderConfig, tc.tokenSvcGetTokenProviderConfigErr
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, tokenSvc, nil, nil, nil, tls.Certificate{})

			// Run test
			err := serverSvc.FactoryResetByName(t.Context(), tc.argName, tc.argTokenID, tc.argTokenSeedName, false)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_GetSystemLogging(t *testing.T) {
	tests := []struct {
		name                      string
		argName                   string
		repoGetByName             *provisioning.Server
		repoGetByNameErr          error
		clientGetSystemLogging    provisioning.ServerSystemLogging
		clientGetSystemLoggingErr error

		assertErr         require.ErrorAssertionFunc
		wantLoggingConfig provisioning.ServerSystemLogging
	}{
		{
			name:    "success",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemLogging: incusosapi.SystemLogging{
				Config: incusosapi.SystemLoggingConfig{
					Syslog: incusosapi.SystemLoggingSyslog{
						Address: "localhost",
					},
				},
			},

			assertErr: require.NoError,
			wantLoggingConfig: incusosapi.SystemLogging{
				Config: incusosapi.SystemLoggingConfig{
					Syslog: incusosapi.SystemLoggingSyslog{
						Address: "localhost",
					},
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.GetSystemLogging",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemLoggingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetSystemLoggingFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemLogging, error) {
					return tc.clientGetSystemLogging, tc.clientGetSystemLoggingErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			loggingConfig, err := serverSvc.GetSystemLogging(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantLoggingConfig, loggingConfig)
		})
	}
}

func TestServerService_UpdateSystemLogging(t *testing.T) {
	tests := []struct {
		name                         string
		argName                      string
		argLoggingConfig             incusosapi.SystemLogging
		repoGetByName                *provisioning.Server
		repoGetByNameErr             error
		clientUpdateSystemLoggingErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:             "success",
			argName:          "one",
			argLoggingConfig: incusosapi.SystemLogging{},
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.UpdateSystemLogging",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientUpdateSystemLoggingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateSystemLoggingFunc: func(ctx context.Context, server provisioning.Server, config provisioning.ServerSystemLogging) error {
					return tc.clientUpdateSystemLoggingErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemLogging(t.Context(), tc.argName, tc.argLoggingConfig)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_GetSystemKernel(t *testing.T) {
	tests := []struct {
		name                     string
		argName                  string
		repoGetByName            *provisioning.Server
		repoGetByNameErr         error
		clientGetSystemKernel    provisioning.ServerSystemKernel
		clientGetSystemKernelErr error

		assertErr        require.ErrorAssertionFunc
		wantKernelConfig provisioning.ServerSystemKernel
	}{
		{
			name:    "success",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemKernel: incusosapi.SystemKernel{
				Config: incusosapi.SystemKernelConfig{
					BlacklistModules: []string{"foobar"},
				},
			},

			assertErr: require.NoError,
			wantKernelConfig: incusosapi.SystemKernel{
				Config: incusosapi.SystemKernelConfig{
					BlacklistModules: []string{"foobar"},
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.GetSystemKernel",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemKernelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetSystemKernelFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemKernel, error) {
					return tc.clientGetSystemKernel, tc.clientGetSystemKernelErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			kernelConfig, err := serverSvc.GetSystemKernel(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantKernelConfig, kernelConfig)
		})
	}
}

func TestServerService_UpdateSystemKernel(t *testing.T) {
	tests := []struct {
		name                        string
		argName                     string
		argKernelConfig             incusosapi.SystemKernel
		repoGetByName               *provisioning.Server
		repoGetByNameErr            error
		clientUpdateSystemKernelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "success",
			argName:         "one",
			argKernelConfig: incusosapi.SystemKernel{},
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.UpdateSystemKernel",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientUpdateSystemKernelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateSystemKernelFunc: func(ctx context.Context, server provisioning.Server, config provisioning.ServerSystemKernel) error {
					return tc.clientUpdateSystemKernelErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemKernel(t.Context(), tc.argName, tc.argKernelConfig)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_AddApplication(t *testing.T) {
	tests := []struct {
		name                    string
		argName                 string
		argApplicationName      string
		repoGetByName           *provisioning.Server
		repoGetByNameErr        error
		clientAddApplicationErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success",
			argName:            "one",
			argApplicationName: "debug",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.AddApplication",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientAddApplicationErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				AddApplicationFunc: func(ctx context.Context, server provisioning.Server, application string) error {
					return tc.clientAddApplicationErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.AddApplication(t.Context(), tc.argName, tc.argApplicationName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_RestartApplication(t *testing.T) {
	tests := []struct {
		name                        string
		argName                     string
		argApplicationName          string
		repoGetByName               *provisioning.Server
		repoGetByNameErr            error
		clientRestartApplicationErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success",
			argName:            "one",
			argApplicationName: "debug",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.RestartApplication",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientRestartApplicationErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				RestartApplicationFunc: func(ctx context.Context, server provisioning.Server, application string) error {
					return tc.clientRestartApplicationErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.RestartApplication(t.Context(), tc.argName, tc.argApplicationName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_ResyncBMCData(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name string

		repoGetAllServers provisioning.Servers
		repoGetAllErr     error

		bmcClientGetData    api.BMCData
		bmcClientGetDataErr error

		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error
		repoUpdateErr       error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - no servers",

			assertErr: require.NoError,
		},
		{
			name: "success - server without BMC type configured",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeNone,
						Endpoint: "https://bmc.local",
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - server with BMC type but no endpoint",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name: "success - resync",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name:          "error - repo.GetAll",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - no BMC server client registered for type",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPIType("unknown"),
						Endpoint: "https://bmc.local",
					},
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name: "error - client.GetServerDetails",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetDataErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.GetByName",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.Update",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.repoGetAllServers, tc.repoGetAllErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)
					require.Equal(t, &wantDetails.ServerUUID, in.SystemUUID)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.ResyncBMCData(t.Context())

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCServerPowerOnByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                      string
		nameArg                   string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		bmcClientServerPowerOnErr error
		bmcClientWaitErr          error
		bmcClientGetData          api.BMCData
		bmcClientGetDataErr       error
		repoUpdateErr             error
		resyncDone                chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - task monitor wait fails but resync still runs",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - resync fails",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,
			resyncDone:          make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:       "error - name empty",
			nameArg:    "", // invalid
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,
			resyncDone:       closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ServerPowerOn",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerPowerOnErr: boom.Error,
			resyncDone:                closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerPowerOnFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
					return taskMonitor, tc.bmcClientServerPowerOnErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerPowerOnByName(t.Context(), tc.nameArg, false)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}

func TestServerService_BMCServerPowerOffByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                       string
		nameArg                    string
		repoGetByNameServer        *provisioning.Server
		repoGetByNameErr           error
		bmcClientServerPowerOffErr error
		bmcClientWaitErr           error
		bmcClientGetData           api.BMCData
		bmcClientGetDataErr        error
		repoUpdateErr              error
		resyncDone                 chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - task monitor wait fails but resync still runs",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - resync fails",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,
			resyncDone:          make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:       "error - name empty",
			nameArg:    "", // invalid
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,
			resyncDone:       closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ServerPowerOff",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerPowerOffErr: boom.Error,
			resyncDone:                 closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerPowerOffFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
					return taskMonitor, tc.bmcClientServerPowerOffErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerPowerOffByName(t.Context(), tc.nameArg, false)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}

func TestServerService_BMCServerRestartByName(t *testing.T) {
	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	tests := []struct {
		name                      string
		nameArg                   string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		bmcClientServerRestartErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ServerRestart",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerRestartErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerRestartFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
					return taskMonitor, tc.bmcClientServerRestartErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerRestartByName(t.Context(), tc.nameArg, false)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCServerSetLocationIndicatorByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                                   string
		nameArg                                string
		activeArg                              bool
		repoGetByNameServer                    *provisioning.Server
		repoGetByNameErr                       error
		bmcClientServerSetLocationIndicatorErr error
		bmcClientGetData                       api.BMCData
		bmcClientGetDataErr                    error
		repoUpdateErr                          error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:      "success - turn on",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerLocationIndicatorActive: true,
			},

			assertErr: require.NoError,
		},
		{
			name:      "success - turn off",
			nameArg:   "one",
			activeArg: false,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerLocationIndicatorActive: false,
			},

			assertErr: require.NoError,
		},
		{
			name:      "success - resync fails",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,

			assertErr: require.NoError,
		},
		{
			name:      "success - repo.Update fails during resync",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:      "error - client.ServerSetLocationIndicator",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerSetLocationIndicatorErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerSetLocationIndicatorFunc: func(ctx context.Context, server provisioning.Server, active bool) error {
					require.Equal(t, tc.activeArg, active)

					return tc.bmcClientServerSetLocationIndicatorErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerSetLocationIndicatorByName(t.Context(), tc.nameArg, tc.activeArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_ApplyBIOSAttributesByName(t *testing.T) {
	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                            string
		nameArg                         string
		attributesArg                   map[string]any
		repoGetByNameServer             *provisioning.Server
		repoGetByNameErr                error
		bmcClientApplyBIOSAttributesErr error
		bmcClientWaitErr                error
		waitDone                        chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success",
			nameArg:       "one",
			attributesArg: map[string]any{"SecureBoot": "Enabled"},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			waitDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:          "success - wait for task fails",
			nameArg:       "one",
			attributesArg: map[string]any{"SecureBoot": "Enabled"},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			waitDone:         make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:     "error - name empty",
			nameArg:  "", // invalid
			waitDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,
			waitDone:         closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			waitDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:          "error - client.ApplyBIOSAttributes",
			nameArg:       "one",
			attributesArg: map[string]any{"SecureBoot": "Enabled"},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientApplyBIOSAttributesErr: boom.Error,
			waitDone:                        closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ApplyBIOSAttributesFunc: func(ctx context.Context, server provisioning.Server, attributes map[string]any) (*provisioning.BMCTaskMonitor, error) {
					require.Equal(t, tc.attributesArg, attributes)

					return taskMonitor, tc.bmcClientApplyBIOSAttributesErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					defer close(tc.waitDone)

					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.ApplyBIOSAttributesByName(t.Context(), tc.nameArg, tc.attributesArg)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.waitDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC task wait")
			}
		})
	}
}

func TestServerService_BMCBIOSAttributesByName(t *testing.T) {
	attributes := []api.BIOSAttribute{
		{Name: "NumaNodesPerSocket", Type: "String", CurrentValue: "4"},
		{Name: "SecureBoot", Type: "Enumeration", CurrentValue: "Enabled"},
	}

	tests := []struct {
		name                       string
		nameArg                    string
		repoGetByNameServer        *provisioning.Server
		repoGetByNameErr           error
		bmcClientBIOSAttributesErr error

		assertErr require.ErrorAssertionFunc
		want      []api.BIOSAttribute
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      attributes,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.BIOSAttributes",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientBIOSAttributesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
					return attributes, tc.bmcClientBIOSAttributesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			got, err := serverSvc.BMCBIOSAttributesByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_BIOSProfileByName(t *testing.T) {
	resolution := provisioning.BIOSProfileResolution{
		Profiles: []string{"dell-poweredge"},
		Attributes: map[string]any{
			"SecureBoot": "Enabled",
		},
	}

	tests := []struct {
		name                   string
		nameArg                string
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		biosProfileResolve     *provisioning.BIOSProfileResolution
		biosProfileResolveErr  error
		withoutBIOSProfilePort bool

		assertErr require.ErrorAssertionFunc
		want      *provisioning.BIOSProfileResolution
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			biosProfileResolve: &resolution,

			assertErr: require.NoError,
			want:      &resolution,
		},
		{
			name:    "success - no profile matches",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BIOS profile port configured",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			withoutBIOSProfilePort: true,

			assertErr: errassert.NotFoundError,
		},
		{
			name:    "error - biosProfile.Resolve",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			biosProfileResolveErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			opts := []provisioningServer.Option{}
			if !tc.withoutBIOSProfilePort {
				opts = append(opts, provisioningServer.WithBIOSProfilePort(&adapterMock.BIOSProfilePortMock{
					ResolveFunc: func(ctx context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
						return tc.biosProfileResolve, tc.biosProfileResolveErr
					},
				}))
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				opts...,
			)

			// Run test
			got, err := serverSvc.BIOSProfileByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_ValidateBIOSProfileByName(t *testing.T) {
	resolution := provisioning.BIOSProfileResolution{
		Profiles: []string{"dell-poweredge"},
		Attributes: map[string]any{
			"SecureBoot": "Enabled",
		},
	}

	tests := []struct {
		name                       string
		nameArg                    string
		repoGetByNameServer        *provisioning.Server
		biosProfileResolve         *provisioning.BIOSProfileResolution
		bmcClientBIOSAttributes    []api.BIOSAttribute
		bmcClientBIOSAttributesErr error

		assertErr require.ErrorAssertionFunc
		want      *provisioning.BIOSProfileResolution
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			biosProfileResolve: &resolution,
			bmcClientBIOSAttributes: []api.BIOSAttribute{
				{Name: "SecureBoot", Type: "Enumeration", AcceptableValues: []string{"Enabled", "Disabled"}},
			},

			assertErr: require.NoError,
			want:      &resolution,
		},
		{
			name:    "success - no profile matches",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:    "error - attribute not accepted by the BMC",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			biosProfileResolve: &resolution,
			bmcClientBIOSAttributes: []api.BIOSAttribute{
				{Name: "SecureBoot", Type: "Enumeration", AcceptableValues: []string{"Disabled"}},
			},

			assertErr: errassert.ValidationErrorContains(`"SecureBoot"`),
		},
		{
			name:    "error - client.BIOSAttributes",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			biosProfileResolve:         &resolution,
			bmcClientBIOSAttributesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, nil
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
					return tc.bmcClientBIOSAttributes, tc.bmcClientBIOSAttributesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
				provisioningServer.WithBIOSProfilePort(&adapterMock.BIOSProfilePortMock{
					ResolveFunc: func(ctx context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
						return tc.biosProfileResolve, nil
					},
				}),
			)

			// Run test
			got, err := serverSvc.ValidateBIOSProfileByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_BMCBIOSAttributeAcceptableValuesByNameAndAttribute(t *testing.T) {
	values := api.BIOSAttribute{
		CurrentValue:     "Enabled",
		AcceptableValues: []string{"Enabled", "Disabled"},
	}

	tests := []struct {
		name                      string
		nameArg                   string
		attributeNameArg          string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		bmcClientBIOSAttributeErr error

		assertErr require.ErrorAssertionFunc
		want      api.BIOSAttribute
	}{
		{
			name:             "success",
			nameArg:          "one",
			attributeNameArg: "SecureBoot",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      values,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:             "error - client.BIOSAttribute",
			nameArg:          "one",
			attributeNameArg: "SecureBoot",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientBIOSAttributeErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				BIOSAttributeFunc: func(ctx context.Context, server provisioning.Server, attributeName string) (api.BIOSAttribute, error) {
					require.Equal(t, tc.attributeNameArg, attributeName)

					return values, tc.bmcClientBIOSAttributeErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			got, err := serverSvc.BMCBIOSAttributeByName(t.Context(), tc.nameArg, tc.attributeNameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_BMCApplySecureBootCertificatesByName(t *testing.T) {
	tests := []struct {
		name                                    string
		nameArg                                 string
		repoGetByNameServer                     *provisioning.Server
		repoGetByNameErr                        error
		bmcClientApplySecureBootCertificatesErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ApplySecureBootCertificates",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientApplySecureBootCertificatesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ApplySecureBootCertificatesFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.bmcClientApplySecureBootCertificatesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCApplySecureBootCertificatesByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCLogSourcesByName(t *testing.T) {
	logSources := []string{"chassis/Logs", "manager/SEL", "system/Logs"}

	tests := []struct {
		name                   string
		nameArg                string
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		bmcClientLogSourcesErr error

		assertErr require.ErrorAssertionFunc
		want      []string
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      logSources,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.LogSources",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientLogSourcesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				LogSourcesFunc: func(ctx context.Context, server provisioning.Server) ([]string, error) {
					return logSources, tc.bmcClientLogSourcesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			gotLogSources, err := serverSvc.BMCLogSourcesByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, gotLogSources)
		})
	}
}

func TestServerService_BMCLogEntriesByNameAndLogSource(t *testing.T) {
	logEntries := []api.BMCLogEvent{
		{
			EntryType: "SEL",
			Message:   "A log message",
			Severity:  "OK",
		},
	}

	tests := []struct {
		name                   string
		nameArg                string
		logSourceArg           string
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		bmcClientLogEntriesErr error

		assertErr require.ErrorAssertionFunc
		want      []api.BMCLogEvent
	}{
		{
			name:         "success",
			nameArg:      "one",
			logSourceArg: "chassis/Logs",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      logEntries,
		},
		{
			name:         "error - name empty",
			nameArg:      "", // invalid
			logSourceArg: "chassis/Logs",

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:         "error - log source empty",
			nameArg:      "one",
			logSourceArg: "", // invalid

			assertErr: errassert.OperationNotPermittedErrorContains(`Log source "" must have the structure "service/logService"`),
		},
		{
			name:         "error - log source without log service",
			nameArg:      "one",
			logSourceArg: "chassis", // invalid

			assertErr: errassert.OperationNotPermittedErrorContains(`Log source "chassis" must have the structure "service/logService"`),
		},
		{
			name:         "error - log source with empty log service",
			nameArg:      "one",
			logSourceArg: "chassis/", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:         "error - log source with too many parts",
			nameArg:      "one",
			logSourceArg: "chassis/Logs/Entries", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			logSourceArg:     "chassis/Logs",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:         "error - no BMC server client registered for type",
			nameArg:      "one",
			logSourceArg: "chassis/Logs",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:         "error - client.LogEntriesBySource",
			nameArg:      "one",
			logSourceArg: "chassis/Logs",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientLogEntriesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				LogEntriesBySourceFunc: func(ctx context.Context, server provisioning.Server, logSource string) ([]api.BMCLogEvent, error) {
					return logEntries, tc.bmcClientLogEntriesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			gotLogEntries, err := serverSvc.BMCLogEntriesByNameAndLogSource(t.Context(), tc.nameArg, tc.logSourceArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, gotLogEntries)
		})
	}
}

func TestServerService_BMCDumpByName(t *testing.T) {
	dump := api.BMCDump{
		"/redfish/v1/": api.BMCDumpEntry{
			Response: json.RawMessage(`{"Id":"RootService"}`),
		},
	}

	tests := []struct {
		name                   string
		nameArg                string
		additionalEndpointsArg []string
		skipPredefinedArg      bool
		traceArg               bool
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		bmcClientDumpErr       error

		assertErr require.ErrorAssertionFunc
		want      api.BMCDump
	}{
		{
			name:                   "success",
			nameArg:                "one",
			additionalEndpointsArg: []string{"/redfish/v1/Systems/1/Oem/Vendor"},
			skipPredefinedArg:      true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      dump,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.Dump",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientDumpErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				DumpFunc: func(ctx context.Context, server provisioning.Server, additionalEndpoints []string, skipPredefined bool, trace bool) (api.BMCDump, error) {
					return dump, tc.bmcClientDumpErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			gotDump, err := serverSvc.BMCDumpByName(t.Context(), tc.nameArg, tc.additionalEndpointsArg, tc.skipPredefinedArg, tc.traceArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, gotDump)

			if tc.repoGetByNameServer != nil && tc.repoGetByNameServer.BMCConfig.APIType == api.BMCAPITypeRedfishV1Generic {
				require.Len(t, bmcClient.DumpCalls(), 1)
				require.Equal(t, tc.additionalEndpointsArg, bmcClient.DumpCalls()[0].AdditionalEndpoints)
				require.Equal(t, tc.skipPredefinedArg, bmcClient.DumpCalls()[0].SkipPredefined)
			}
		})
	}
}

func TestServerService_SyncCluster(t *testing.T) {
	s := provisioningServer.New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{})
	err := s.SyncCluster(t.Context(), "")
	require.NoError(t, err)
}

func TestServerService_BMCRefreshByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name    string
		nameArg string

		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error

		bmcClientGetData    api.BMCData
		bmcClientGetDataErr error

		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPIType("unknown"),
					Endpoint: "https://bmc.local",
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.GetData",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetDataErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.Update",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
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
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)
					require.Equal(t, &wantDetails.ServerUUID, in.SystemUUID)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCRefreshByName(t.Context(), tc.nameArg)

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

func TestServerService_BMCAttachMediaByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	const tokenUUID = "e9de436e-b94e-4aef-8563-883aec84096e"

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	server := &provisioning.Server{
		Name: "one",
		BMCConfig: api.BMCConfig{
			APIType: api.BMCAPITypeRedfishV1Generic,
		},
	}

	tests := []struct {
		name                      string
		nameArg                   string
		mediaArg                  api.ServerBMCAttachMedia
		operationsCenterAddress   string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		tokenSvcGetSeed           *provisioning.TokenSeed
		tokenSvcGetSeedErr        error
		tokenSvcResolveImageIDErr error
		channelSvcGetByNameErr    error
		bmcClientAttachMediaErr   error
		bmcClientWaitErr          error
		bmcClientGetDataErr       error
		repoUpdateErr             error
		resyncDone                chan struct{}

		wantMediaURL       string
		wantVirtualMediaID string
		assertErr          require.ErrorAssertionFunc
	}{
		{
			name:    "success - without channel",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - with boot device",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
				SetBootDevice:  true,
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - with channel",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "raw",
				Architecture:   "aarch64",
				Channel:        "stable",
				VirtualMediaID: "manager:2",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/aarch64/channel/stable/type/raw/" + testSeedImageID + ".raw",
			wantVirtualMediaID: "manager:2",
			assertErr:          require.NoError,
		},
		{
			name:    "success - channel needs URL escaping",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				Channel:        "team/beta 2",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/channel/team%2Fbeta%202/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - task monitor wait fails but resync still runs",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			bmcClientWaitErr:        boom.Error,
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - resync fails to get BMC data",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			bmcClientGetDataErr:     boom.Error,
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - resync fails to update the server",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			repoUpdateErr:           boom.Error,
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - seed empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "", // invalid
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - virtual media ID empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				VirtualMediaID: "", // invalid
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - invalid token UUID",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      "not-a-uuid", // invalid
				Seed:           "default",
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - invalid image type",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "qcow2", // invalid
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid image type "qcow2"`),
		},
		{
			name:    "error - image type empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "", // invalid
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid image type ""`),
		},
		{
			name:    "error - invalid architecture",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "riscv64", // invalid
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid architecture "riscv64"`),
		},
		{
			name:    "error - architecture empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "", // invalid, undefined architecture
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid architecture ""`),
		},
		{
			name:    "error - channel does not exist",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				Channel:        "does-not-exist",
				VirtualMediaID: "system:1",
			},
			channelSvcGetByNameErr: boom.Error,
			resyncDone:             closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - token seed not found",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			tokenSvcGetSeedErr: boom.Error,
			resyncDone:         closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - token seed not public",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			tokenSvcGetSeed: &provisioning.TokenSeed{Name: "default", Public: false},
			resyncDone:      closedChannel(),

			assertErr: errassert.Contains("must be public"),
		},
		{
			name:    "error - Operations Center address not configured",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "", // not configured
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              closedChannel(),

			assertErr: errassert.Contains("Operations Center address is not configured"),
		},
		{
			name:    "error - repo.GetByName",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameErr:        boom.Error,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			tokenSvcGetSeed: &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:      closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.AttachMedia",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			bmcClientAttachMediaErr: boom.Error,
			resyncDone:              closedChannel(),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)

			if tc.operationsCenterAddress != "" {
				err := config.UpdateNetwork(t.Context(), system.NetworkPut{
					OperationsCenterAddress: tc.operationsCenterAddress,
					RestServerAddress:       "[::]:8443",
				})
				require.NoError(t, err)
			}

			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					return tc.repoUpdateErr
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					require.Equal(t, tc.mediaArg.Seed, name)

					return tc.tokenSvcGetSeed, tc.tokenSvcGetSeedErr
				},
				ResolveTokenSeedImageIDFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
					return testSeedImageID, tc.tokenSvcResolveImageIDErr
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					require.Equal(t, tc.mediaArg.Channel, name)

					return &provisioning.Channel{Name: name}, tc.channelSvcGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				AttachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
					require.Equal(t, tc.wantVirtualMediaID, virtualMediaID)
					require.Equal(t, tc.wantMediaURL, mediaURL)
					require.Equal(t, tc.mediaArg.SetBootDevice, setBootDevice)

					return taskMonitor, tc.bmcClientAttachMediaErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return api.BMCData{}, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, tokenSvc, nil, channelSvc, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCAttachMediaByName(t.Context(), tc.nameArg, tc.mediaArg)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}

func TestServerService_BMCVirtualMediaSignal(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	const tokenUUID = "e9de436e-b94e-4aef-8563-883aec84096e"

	server := &provisioning.Server{
		Name: "one",
		BMCConfig: api.BMCConfig{
			APIType: api.BMCAPITypeRedfishV1Generic,
		},
	}

	setup := func(t *testing.T) (provisioning.ServerService, chan lifecycle.BMCVirtualMediaMessage, chan struct{}, *adapterMock.BMCServerClientPortMock) {
		t.Helper()

		config.InitTest(t, &envMock.EnvironmentMock{
			IsIncusOSFunc: func() bool {
				return false
			},
		}, nil)

		err := config.UpdateNetwork(t.Context(), system.NetworkPut{
			OperationsCenterAddress: "https://192.168.1.200:8443",
			RestServerAddress:       "[::]:8443",
		})
		require.NoError(t, err)

		resyncDone := make(chan struct{})

		repo := &repoMock.ServerRepoMock{
			GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
				return server, nil
			},
			UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
				defer close(resyncDone)

				return nil
			},
		}

		tokenSvc := &svcMock.TokenServiceMock{
			GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
				return &provisioning.TokenSeed{Name: name, Public: true}, nil
			},
			ResolveTokenSeedImageIDFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
				return testSeedImageID, nil
			},
		}

		channelSvc := &svcMock.ChannelServiceMock{
			GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
				return &provisioning.Channel{Name: name}, nil
			},
		}

		bmcClient := &adapterMock.BMCServerClientPortMock{
			AttachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
				return nil, nil
			},
			DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
				return nil, nil
			},
			WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
				return nil
			},
			GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
				return api.BMCData{}, nil
			},
		}

		serverSvc := provisioningServer.New(
			repo, nil, nil, tokenSvc, nil, channelSvc, nil, tls.Certificate{},
			provisioningServer.WithNow(func() time.Time { return fixedDate }),
			provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
		)

		messages := make(chan lifecycle.BMCVirtualMediaMessage, 8)

		lifecycle.BMCVirtualMediaSignal.AddListener(func(ctx context.Context, msg lifecycle.BMCVirtualMediaMessage) {
			messages <- msg
		})

		t.Cleanup(lifecycle.BMCVirtualMediaSignal.Reset)

		t.Cleanup(serverSvc.WaitBackgroundTasks)

		return serverSvc, messages, resyncDone, bmcClient
	}

	awaitMessage := func(t *testing.T, messages chan lifecycle.BMCVirtualMediaMessage) lifecycle.BMCVirtualMediaMessage {
		t.Helper()

		select {
		case msg := <-messages:
			return msg

		case <-time.After(time.Second):
			t.Fatal("timed out waiting for the virtual media signal")
		}

		return lifecycle.BMCVirtualMediaMessage{}
	}

	awaitResync := func(t *testing.T, resyncDone chan struct{}) {
		t.Helper()

		select {
		case <-resyncDone:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for asynchronous BMC resync")
		}
	}

	t.Run("attach reports everything needed to prepare the image", func(t *testing.T) {
		serverSvc, messages, resyncDone, _ := setup(t)

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "iso",
			Architecture:   "x86_64",
			Channel:        "stable",
			VirtualMediaID: "system:1",
		})
		require.NoError(t, err)

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationPreAttach,
			Server:         "one",
			VirtualMediaID: "system:1",
			TokenUUID:      uuid.MustParse(tokenUUID),
			Seed:           "default",
			ImageType:      api.ImageTypeISO,
			Architecture:   images.UpdateFileArchitecture64BitX86,
			Channel:        "stable",
		}, awaitMessage(t, messages))

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationAttach,
			Server:         "one",
			VirtualMediaID: "system:1",
			TokenUUID:      uuid.MustParse(tokenUUID),
			Seed:           "default",
			ImageType:      api.ImageTypeISO,
			Architecture:   images.UpdateFileArchitecture64BitX86,
			Channel:        "stable",
		}, awaitMessage(t, messages))

		awaitResync(t, resyncDone)
	})

	t.Run("detach reports the virtual media it has been detached from", func(t *testing.T) {
		serverSvc, messages, resyncDone, _ := setup(t)

		err := serverSvc.BMCDetachMediaByName(t.Context(), "one", "system:1")
		require.NoError(t, err)

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationDetach,
			Server:         "one",
			VirtualMediaID: "system:1",
		}, awaitMessage(t, messages))

		awaitResync(t, resyncDone)
	})

	t.Run("the preparation is reported before the BMC is instructed", func(t *testing.T) {
		serverSvc, messages, resyncDone, bmcClient := setup(t)

		var reportedBeforeAttach []lifecycle.BMCVirtualMediaMessage

		bmcClient.AttachMediaFunc = func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
			for len(messages) > 0 {
				reportedBeforeAttach = append(reportedBeforeAttach, <-messages)
			}

			return nil, nil
		}

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "iso",
			Architecture:   "x86_64",
			Channel:        "stable",
			VirtualMediaID: "system:1",
		})
		require.NoError(t, err)

		require.Len(t, reportedBeforeAttach, 1, "the installation media has to be reported before the BMC is instructed to attach it")
		require.Equal(t, lifecycle.BMCVirtualMediaOperationPreAttach, reportedBeforeAttach[0].Operation)

		awaitResync(t, resyncDone)
	})

	t.Run("a failed attach only reports the preparation", func(t *testing.T) {
		serverSvc, messages, _, bmcClient := setup(t)

		bmcClient.AttachMediaFunc = func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		}

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "iso",
			Architecture:   "x86_64",
			Channel:        "stable",
			VirtualMediaID: "system:1",
		})
		boom.ErrorIs(t, err)

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationPreAttach,
			Server:         "one",
			VirtualMediaID: "system:1",
			TokenUUID:      uuid.MustParse(tokenUUID),
			Seed:           "default",
			ImageType:      api.ImageTypeISO,
			Architecture:   images.UpdateFileArchitecture64BitX86,
			Channel:        "stable",
		}, awaitMessage(t, messages))

		require.Empty(t, messages)
	})

	t.Run("a rejected attach request reports nothing", func(t *testing.T) {
		serverSvc, messages, _, _ := setup(t)

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "exe",
			Architecture:   "x86_64",
			VirtualMediaID: "system:1",
		})
		require.Error(t, err)
		require.Empty(t, messages)
	})
}

func TestServerService_BMCDetachMediaByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                    string
		nameArg                 string
		virtualMediaIDArg       string
		repoGetByNameServer     *provisioning.Server
		repoGetByNameErr        error
		bmcClientDetachMediaErr error
		bmcClientWaitErr        error
		bmcClientGetDataErr     error
		repoUpdateErr           error
		resyncDone              chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:              "success",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "success - task monitor wait fails but resync still runs",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			resyncDone:       make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "success - resync fails to get BMC data",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,
			resyncDone:          make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "success - resync fails to update the server",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			repoUpdateErr: boom.Error,
			resyncDone:    make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "error - name empty",
			nameArg:           "", // invalid
			virtualMediaIDArg: "system:1",
			resyncDone:        closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:              "error - virtual media ID empty",
			nameArg:           "one",
			virtualMediaIDArg: "", // invalid
			resyncDone:        closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:              "error - repo.GetByName",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameErr:  boom.Error,
			resyncDone:        closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:              "error - no BMC server client registered for type",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:              "error - client.DetachMedia",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientDetachMediaErr: boom.Error,
			resyncDone:              closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
					require.Equal(t, tc.virtualMediaIDArg, virtualMediaID)

					return taskMonitor, tc.bmcClientDetachMediaErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return api.BMCData{}, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCDetachMediaByName(t.Context(), tc.nameArg, tc.virtualMediaIDArg)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}
