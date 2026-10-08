package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusclient "github.com/lxc/incus/v7/client"
	incusapi "github.com/lxc/incus/v7/shared/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

const testCertificate = `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`

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

			assertErr:    errassert.ValidationError,
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

		assertErr      require.ErrorAssertionFunc
		wantSystemUUID *string
		wantMachineID  *string
	}{
		{
			name: "success - new registration",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
			},

			assertErr: require.NoError,
		},
		{
			name: "success - pre registered server by system UUID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				SystemUUID:    new("1"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:         1,
				Name:       "one",
				Status:     api.ServerStatusUnregistered,
				SystemUUID: new("1"),
			},

			assertErr: require.NoError,
		},
		{
			name: "success - pre registered server by machine ID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				MachineID:     new("1"),
			},
			repoGetByMachineID: &provisioning.Server{
				ID:        1,
				Name:      "one",
				Status:    api.ServerStatusUnregistered,
				MachineID: new("1"),
			},

			assertErr: require.NoError,
		},
		{
			name: "success - upper case system UUID is normalized to lower case",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				SystemUUID:    new("E9DE436E-B94E-4AEF-8563-883AEC84096E"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:         1,
				Name:       "one",
				Status:     api.ServerStatusUnregistered,
				SystemUUID: new("e9de436e-b94e-4aef-8563-883aec84096e"),
			},

			assertErr:      require.NoError,
			wantSystemUUID: new("e9de436e-b94e-4aef-8563-883aec84096e"),
		},
		{
			name: "success - upper case system UUID is stored in lower case",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				SystemUUID:    new("E9DE436E-B94E-4AEF-8563-883AEC84096E"),
			},

			assertErr:      require.NoError,
			wantSystemUUID: new("e9de436e-b94e-4aef-8563-883aec84096e"),
		},
		{
			name: "success - upper case machine ID is normalized to lower case",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				MachineID:     new("E9DE436EB94E4AEF8563883AEC84096E"),
			},

			assertErr:     require.NoError,
			wantMachineID: new("e9de436eb94e4aef8563883aec84096e"),
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
				Certificate:   new(testCertificate),
				SystemUUID:    new("1"),
			},
			repoGetBySystemUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - pre registered server by machine ID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				MachineID:     new("1"),
			},
			repoGetByMachineIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - remote Operations Center",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				Type:          api.ServerTypeOperationsCenter,
			},

			assertErr: errassert.ValidationErrorContains("Remote operations centers can not be registered"),
		},
		{
			name: "error - repo.Create",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - matched server already registered",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				SystemUUID:    new("1"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:         1,
				Name:       "one",
				Status:     api.ServerStatusReady,
				SystemUUID: new("1"),
			},

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name: "error - repo.Update - pre registered server by system UUID",
			server: provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				SystemUUID:    new("1"),
			},
			repoGetBySystemUUID: &provisioning.Server{
				ID:         1,
				Name:       "one",
				Status:     api.ServerStatusUnregistered,
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
				Certificate:   new(testCertificate),
			},
			repoUpdateErr: boom.Error,

			assertErr: require.NoError, // Error of connection test is only logged, we can not assert it here.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			assertIdentifiers := func(server provisioning.Server) {
				if tc.wantSystemUUID != nil {
					require.Equal(t, tc.wantSystemUUID, server.SystemUUID, "system UUID should be persisted in lower case")
				}

				if tc.wantMachineID != nil {
					require.Equal(t, tc.wantMachineID, server.MachineID, "machine ID should be persisted in lower case")
				}
			}

			repo := &repoMock.ServerRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.Server) (int64, error) {
					require.Equal(t, fixedDate, in.LastSeen)
					assertIdentifiers(in)
					return 1, tc.repoCreateErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &provisioning.Server{}, nil
				},
				GetBySystemUUIDFunc: func(ctx context.Context, systemUUID string) (*provisioning.Server, error) {
					if tc.wantSystemUUID != nil {
						require.Equal(t, *tc.wantSystemUUID, systemUUID, "lookup by system UUID should use the lower case value")
					}

					return tc.repoGetBySystemUUID, tc.repoGetBySystemUUIDErr
				},
				GetByMachineIDFunc: func(ctx context.Context, machineID string) (*provisioning.Server, error) {
					if tc.wantMachineID != nil {
						require.Equal(t, *tc.wantMachineID, machineID, "lookup by machine ID should use the lower case value")
					}

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

			runner := &adapterMock.ServerScriptletPortMock{
				ServerRegistrationRunFunc: func(ctx context.Context, server *provisioning.Server) error {
					return nil
				},
			}

			token := uuid.MustParse("686d2a12-20f9-11f0-82c6-7fff26bab0c4")

			serverSvc := provisioningServer.New(
				repo, client, runner, tokenSvc, nil, nil, updateSvc, tls.Certificate{},
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
			name: "success - BMC connection test - changes made during the connection test are kept",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusPending,
				Description:   "set by operator",
				Channel:       "stable",
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
						Name:     "one",
						Status:   api.ServerStatusReady,
						LastSeen: fixedDate,
						Channel:  "stable",
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
				require.Equal(t, api.ServerStatusReady, server.Status, "the status set by a poll during the connection test must be kept")
				require.Equal(t, fixedDate, server.LastSeen, "the last seen time set by a poll during the connection test must be kept")
				require.Equal(t, "set by operator", server.Description, "the description of the request must be applied")
				require.Equal(t, certificate, server.BMCConfig.Certificate, "the BMC config of the request must be applied")
			},
		},
		{
			name: "success - BMC connection test - auto pin certificate with provided certificate",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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

			assertErr:           errassert.OperationNotPermittedErrorContains(`The update channel of server "one" is managed by cluster "one"`),
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
				Certificate:   new(testCertificate),
				Status:        api.ServerStatusReady,
				Channel:       "stable",
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
			wantServerStatusDetail: api.ServerStatusDetailReadyUpdatingApplication,
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

			assertErr: errassert.Is(domain.ErrNotAuthorized),
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
				errassert.DomainError(domain.ErrOperationNotPermitted, api.ErrorReasonServerIsClusterMember)(tt, err, a...)
				errassert.UserMessageContains(`Server "one" is a member of cluster "one" and can not be deleted`)(tt, err, a...)
				errassert.HintIs("Remove the server from the cluster first.")(tt, err, a...)
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

func TestServerService_DetachFromCluster(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error
		repoUpdateErr       error

		assertErr    require.ErrorAssertionFunc
		assertServer func(t *testing.T, server *provisioning.Server)
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:                 "one",
				Cluster:              new("clusterOne"),
				ClusterCertificate:   new("cluster certificate"),
				ClusterConnectionURL: new("https://cluster/"),
				Status:               api.ServerStatusOffline,
				StatusDetail:         api.ServerStatusDetailReadyEvacuating,
				StatusInternal: provisioning.ServerStatusInternal{
					Update: &provisioning.ServerUpdate{
						RebootPending: true,
					},
				},
				Description: "a description worth keeping",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			assertServer: func(t *testing.T, server *provisioning.Server) {
				t.Helper()

				require.NotNil(t, server)
				require.Nil(t, server.Cluster)
				require.Nil(t, server.ClusterCertificate)
				require.Nil(t, server.ClusterConnectionURL)
				require.Nil(t, server.StatusInternal.Update)
				require.Equal(t, api.ServerStatusDetailNone, server.StatusDetail)

				// The data provided for the server is preserved.
				require.Equal(t, "a description worth keeping", server.Description)
				require.Equal(t, api.BMCAPITypeRedfishV1Generic, server.BMCConfig.APIType)
			},
		},
		{
			name:    "success - server is not part of a cluster",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:    "one",
				Cluster: nil,
			},

			assertErr: require.NoError,
			assertServer: func(t *testing.T, server *provisioning.Server) {
				t.Helper()

				// The record is left untouched.
				require.Nil(t, server)
			},
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
			name:    "error - repo.Update",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:    "one",
				Cluster: new("clusterOne"),
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var updatedServer *provisioning.Server

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					updatedServer = &server
					return tc.repoUpdateErr
				},
			}

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			err := serverSvc.DetachFromCluster(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)

			if tc.assertServer != nil {
				tc.assertServer(t, updatedServer)
			}
		})
	}
}

func TestServerService_SyncCluster(t *testing.T) {
	s := provisioningServer.New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{})
	err := s.SyncCluster(t.Context(), "")
	require.NoError(t, err)
}

func TestServerService_ReconcileMeshTunnelLocalAddress(t *testing.T) {
	osData := func(addresses ...string) api.OSData {
		return api.OSData{
			Network: incusosapi.SystemNetwork{
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: addresses,
							Roles:     []string{incusosapi.SystemNetworkInterfaceRoleCluster},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name                        string
		serverArg                   provisioning.Server
		clientIncusClientErr        error
		incusClientGetNetwork       incusapi.ConfigMap
		incusClientGetNetworkStatus string
		incusClientGetNetworkErr    error
		incusClientUpdateNetworkErr error

		assertErr           require.ErrorAssertionFunc
		wantMeshTunnelLocal string
	}{
		{
			name: "success - address not yet pinned",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("fd42::2", "192.168.0.100"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
			},

			assertErr:           require.NoError,
			wantMeshTunnelLocal: "192.168.0.100",
		},
		{
			name: "success - pinned address is stale",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
				"tunnel.mesh.local":     "192.168.0.99",
			},

			assertErr:           require.NoError,
			wantMeshTunnelLocal: "192.168.0.100",
		},
		{
			name: "success - pinned address is still assigned",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100", "192.168.0.101"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
				"tunnel.mesh.local":     "192.168.0.101",
			},

			assertErr: require.NoError,
		},
		{
			name: "success - pinned address is not usable",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("fd42::2", "192.168.0.100"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
				"tunnel.mesh.local":     "fd42::2",
			},

			assertErr:           require.NoError,
			wantMeshTunnelLocal: "192.168.0.100",
		},
		{
			name: "success - interface of the network takes precedence",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{"192.168.0.100"},
									Roles:     []string{incusosapi.SystemNetworkInterfaceRoleCluster},
								},
								"eth1": {
									Addresses: []string{"10.0.0.100"},
								},
							},
						},
					},
				},
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth1",
				"tunnel.mesh.local":     "192.168.0.100",
			},

			assertErr:           require.NoError,
			wantMeshTunnelLocal: "10.0.0.100",
		},
		{
			name: "success - network is pending",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
			},
			incusClientGetNetworkStatus: incusapi.NetworkStatusPending,

			assertErr: require.NoError,
		},
		{
			name: "success - network without mesh interface",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{},

			assertErr: require.NoError,
		},
		{
			name: "success - no IPv4 address",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("fd42::2"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
			},

			assertErr: require.NoError,
		},
		{
			name: "success - not clustered",
			serverArg: provisioning.Server{
				Name:   "one",
				OSData: osData("192.168.0.100"),
			},
			incusClientGetNetworkErr: boom.Error, // not called

			assertErr: require.NoError,
		},
		{
			name: "success - no internal mesh network",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			incusClientGetNetworkErr: incusapi.StatusErrorf(http.StatusNotFound, "Network not found"),

			assertErr: require.NoError,
		},
		{
			name: "error - client.IncusClient",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			clientIncusClientErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - incusClient.GetNetwork",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			incusClientGetNetworkErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - incusClient.UpdateNetwork",
			serverArg: provisioning.Server{
				Name:    "one",
				Cluster: new("cluster"),
				OSData:  osData("192.168.0.100"),
			},
			incusClientGetNetwork: incusapi.ConfigMap{
				"tunnel.mesh.interface": "eth0",
			},
			incusClientUpdateNetworkErr: boom.Error,

			assertErr:           boom.ErrorIs,
			wantMeshTunnelLocal: "192.168.0.100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotTarget string
			var gotMeshTunnelLocal string

			var incusClient *adapterMock.InstanceServerMock

			incusClient = &adapterMock.InstanceServerMock{
				UseTargetFunc: func(name string) incusclient.InstanceServer {
					gotTarget = name
					return incusClient
				},
				GetNetworkFunc: func(name string) (*incusapi.Network, string, error) {
					require.Equal(t, "meshbr0", name)

					status := tc.incusClientGetNetworkStatus
					if status == "" {
						status = incusapi.NetworkStatusCreated
					}

					return &incusapi.Network{
						NetworkPut: incusapi.NetworkPut{
							Config: tc.incusClientGetNetwork,
						},
						Status: status,
					}, "etag", tc.incusClientGetNetworkErr
				},
				UpdateNetworkFunc: func(name string, network incusapi.NetworkPut, ETag string) error {
					require.Equal(t, "meshbr0", name)
					require.Equal(t, "etag", ETag)

					gotMeshTunnelLocal = network.Config["tunnel.mesh.local"]

					return tc.incusClientUpdateNetworkErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				IncusClientFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (provisioning.InstanceServer, error) {
					return incusClient, tc.clientIncusClientErr
				},
			}

			serverSvc := provisioningServer.New(nil, client, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			err := serverSvc.ReconcileMeshTunnelLocalAddress(t.Context(), tc.serverArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantMeshTunnelLocal, gotMeshTunnelLocal)

			if len(incusClient.GetNetworkCalls()) > 0 {
				require.Equal(t, tc.serverArg.Name, gotTarget)
			}
		})
	}
}
