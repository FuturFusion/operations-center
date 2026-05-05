package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
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
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
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
				validServer(t, withType(api.ServerTypeOperationsCenter)),
			},

			assertErr: require.NoError,
		},
		{
			name:                 "success - operations center self update - no server of type operations center - trigger self register",
			argCertificate:       serverCertificate,
			repoGetAllWithFilter: provisioning.Servers{},
			repoGetByName:        validServer(t, withType(api.ServerTypeOperationsCenter)),

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
				validServer(t, withName("one")),
				validServer(t, withName("two")),
			},

			assertErr: errassert.Contains(`Invalid internal state, expect at most 1 server of type "operations-center", found 2`),
		},
		// validation error not covered
		{
			name:           "error - operations center self update - repo.Update",
			argCertificate: serverCertificate,
			repoGetAllWithFilter: provisioning.Servers{
				validServer(t, withType(api.ServerTypeOperationsCenter)),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.UpdateServerCertificate(t.Context(), tc.argCertificate)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_Create(t *testing.T) {
	config.InitTest(t, &envMock.EnvironmentMock{}, nil)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name               string
		server             provisioning.Server
		repoCreateErr      error
		tokenSvcConsumeErr error
		repoUpdateErr      error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:   "success",
			server: validServer(t, withType(api.ServerType(""))),

			assertErr: require.NoError,
		},
		{
			name:               "error - token consume",
			tokenSvcConsumeErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:   "error - validation",
			server: validServer(t, withName("")), // invalid empty name

			assertErr: errassert.ValidationError,
		},
		{
			name:   "error - remote Operations Center",
			server: validServer(t, withType(api.ServerTypeOperationsCenter)),

			assertErr: errassert.ValidationErrorContains("Remote operations centers can not be registered"),
		},
		{
			name:          "error - repo.Create",
			server:        validServer(t),
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:          "error - Ping",
			server:        validServer(t),
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

			serverSvc := provisioningServer.New(repo, client, nil, tokenSvc, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithInitialConnectionDelay(0), // Disable delay for initial connection test
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			_, err := serverSvc.Create(t.Context(), token, tc.server)

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
				validServer(t, withName("one")),
				validServer(t, withName("two")),
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
				Cluster: ptr.To("one"),
			},
			repoGetAllWithFilter: provisioning.Servers{
				validServer(t, withName("one")),
				validServer(t, withName("two")),
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`name == "one"`),
			},
			repoGetAllWithFilter: provisioning.Servers{
				validServer(t, withName("one")),
				validServer(t, withName("two")),
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
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},

			assertErr: errassert.ValidationErrorContains("Failed to compile filter expression:"),
			count:     0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`fromBase64("~invalid") == ""`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllWithFilter: provisioning.Servers{
				validServer(t, withName("one")),
			},

			assertErr: errassert.ValidationErrorContains("Failed to execute filter expression:"),
			count:     0,
		},
		{
			name: "error - upodateSvc.GetAllWithFilter",
			filter: provisioning.ServerFilter{
				Cluster: ptr.To("one"),
			},
			repoGetAllWithFilter: provisioning.Servers{
				validServer(t, withName("one")),
				validServer(t, withName("two")),
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
				Cluster: ptr.To("one"),
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
				Expression: ptr.To(`name matches "one"`),
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
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},

			assertErr: errassert.ValidationErrorContains("Failed to compile filter expression:"),
			count:     0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`fromBase64("~invalid") == ""`), // invalid, returns runtime error during evauluation of the expression.
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
			name:                "success - no updates",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t)),

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Channel:       "stable",
				Certificate:   string(certPEM),
				Status:        api.ServerStatusReady,
				Type:          api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					NeedsUpdate:   ptr.To(false),
					NeedsReboot:   ptr.To(false),
					InMaintenance: ptr.To(api.NotInMaintenance),
					OS: api.OSVersionData{
						Name:        "os",
						Version:     "2",
						VersionNext: "2",
						NeedsUpdate: ptr.To(false),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:        "incus",
							Version:     "2",
							NeedsUpdate: ptr.To(false),
						},
						{
							Name:        "incus-ceph",
							Version:     "2",
							NeedsUpdate: ptr.To(false),
						},
					},
				},
			},
		},
		{
			name:                "success - with version data and updates - everything up to date",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t)),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						{
							Component: images.UpdateFileComponentIncusCeph,
						},
					},
				},
				{
					Version: "1",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						{
							Component: images.UpdateFileComponentIncusCeph,
						},
					},
				},
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				Channel:       "stable",
				ConnectionURL: "http://one/",
				Status:        api.ServerStatusReady,
				Certificate:   string(certPEM),
				Type:          api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:             "os",
						Version:          "2",
						VersionNext:      "2",
						AvailableVersion: ptr.To("2"),
						NeedsUpdate:      ptr.To(false),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:             "incus",
							Version:          "2",
							AvailableVersion: ptr.To("2"),
							NeedsUpdate:      ptr.To(false),
						},
						{
							Name:             "incus-ceph",
							Version:          "2",
							AvailableVersion: ptr.To("2"),
							NeedsUpdate:      ptr.To(false),
						},
					},
					NeedsUpdate:   ptr.To(false),
					NeedsReboot:   ptr.To(false),
					InMaintenance: ptr.To(api.NotInMaintenance),
				},
			},
		},
		{
			name:                "success - with version data and updates - update available",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t)),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					Version: "3",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						{
							Component: images.UpdateFileComponentIncusCeph,
						},
					},
				},
				{
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						{
							Component: images.UpdateFileComponentIncusCeph,
						},
					},
				},
				{
					Version: "1",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						{
							Component: images.UpdateFileComponentIncusCeph,
						},
					},
				},
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				Channel:       "stable",
				Certificate:   string(certPEM),
				Status:        api.ServerStatusReady,
				ConnectionURL: "http://one/",
				Type:          api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:             "os",
						Version:          "2",
						VersionNext:      "2",
						AvailableVersion: ptr.To("3"),
						NeedsUpdate:      ptr.To(true),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:             "incus",
							Version:          "2",
							AvailableVersion: ptr.To("3"),
							NeedsUpdate:      ptr.To(true),
						},
						{
							Name:             "incus-ceph",
							Version:          "2",
							AvailableVersion: ptr.To("3"),
							NeedsUpdate:      ptr.To(true),
						},
					},
					NeedsUpdate:   ptr.To(true),
					NeedsReboot:   ptr.To(false),
					InMaintenance: ptr.To(api.NotInMaintenance),
				},
			},
		},
		{
			name:                "success - with version data and updates - no update information for incus-ceph",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t)),
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						// incus-ceph missing here
					},
				},
				{
					Version: "1",
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
						// incus-ceph missing here
					},
				},
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				Channel:       "stable",
				Certificate:   string(certPEM),
				Status:        api.ServerStatusReady,
				ConnectionURL: "http://one/",
				Type:          api.ServerTypeIncus,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:             "os",
						Version:          "2",
						VersionNext:      "2",
						AvailableVersion: ptr.To("2"),
						NeedsUpdate:      ptr.To(false),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:             "incus",
							Version:          "2",
							AvailableVersion: ptr.To("2"),
							NeedsUpdate:      ptr.To(false),
						},
						{
							Name:        "incus-ceph",
							Version:     "2",
							NeedsUpdate: ptr.To(false),
						},
					},
					NeedsUpdate:   ptr.To(false),
					NeedsReboot:   ptr.To(false),
					InMaintenance: ptr.To(api.NotInMaintenance),
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
			name:                         "error - updateSvc.GetAllWithFilter",
			nameArg:                      "one",
			repoGetByNameServer:          ptr.To(validServer(t)),
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

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{},
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

	tests := []struct {
		name           string
		argForce       bool
		server         provisioning.Server
		repoUpdateErrs queue.Errs
		repoGetByName  []queue.Item[*provisioning.Server]

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name:   "success",
			server: validServer(t),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t)),
				},
				{
					Value: ptr.To(validServer(t)),
				},
				{
					Value: ptr.To(validServer(t)),
				},
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:   "error - validation",
			server: validServer(t, withName("")), // invalid empty name

			assertErr: errassert.ValidationError,
			assertLog: log.Noop,
		},
		{
			name:     "error - repo.GetByName - without force",
			argForce: false,
			server:   validServer(t),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:     "error - channel update for clustered server",
			argForce: false,
			server:   validServer(t),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withChannel("testing"))),
				},
			},

			assertErr: errassert.OperationNotPermittedErrorContains(`Update of channel not allowed for clustered server "one"`),
			assertLog: log.Noop,
		},
		{
			name:   "error - repo.UpdateByID",
			server: validServer(t),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t)),
				},
			},
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:     "error - repo.GetByName - force", // UpdateSystemUpdate
			argForce: true,
			server:   validServer(t),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t)),
				},
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:     "error - repo.GetByName - force - revert error", // UpdateSystemUpdate
			argForce: true,
			server:   validServer(t),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t)),
				},
				{
					Err: boom.Error,
				},
			},
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Contains("Failed to restore previous server state after failed to update system update config"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					return tc.repoUpdateErrs.PopOrNil(t)
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return queue.Pop(t, &tc.repoGetByName)
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, channelSvc, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err = serverSvc.Update(t.Context(), tc.server, tc.argForce, true)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

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
			name:                "success",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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
			name:                "error - repo.UpdateByID",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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
			repoGetByNameServer: validServer(t),
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
			name:                "error - client.UpdateNetworkConfig",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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
			name:                "error - client.UpdateNetworkConfig - reverter error",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
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
			name:                "success",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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
			name:                "error - repo.UpdateByID",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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
			repoGetByNameServer: validServer(t),
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
			name:                "error - client.UpdateStorageConfig",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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
			name:                "error - client.UpdateStorageConfig - reverter error",
			ctx:                 t.Context(),
			repoGetByNameServer: validServer(t),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
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
			name:                "success",
			repoGetByNameServer: validServer(t),
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
			name:                       "error - client.GetProviderConfig",
			repoGetByNameServer:        validServer(t),
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
			name:                "success",
			repoGetByNameServer: validServer(t),

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                          "error - client.UpdateProviderConfig",
			repoGetByNameServer:           validServer(t),
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
			err := serverSvc.UpdateSystemProvider(t.Context(), "one", incusosapi.SystemProvider{
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
			name:                "success",
			repoGetByNameServer: validServer(t),
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
			name:                     "error - client.GetUpdateConfig",
			repoGetByNameServer:      validServer(t),
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
			name:                "success",
			repoGetByNameServer: validServer(t),
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
			name:                "error - client.UpdateUpdateConfig",
			repoGetByNameServer: validServer(t),
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
			err := serverSvc.UpdateSystemUpdate(t.Context(), "one", incusosapi.SystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     true,
					Channel:        "testing",
					CheckFrequency: "2h",
				},
			},
			)

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
			name:                "success",
			repoGetByNameServer: validServer(t),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
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

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name: "success",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t)),

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name: "success - cause network config changed",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCauseNetworkConfigChanged,
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t)),

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name: "success - with other cause",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				Cause:                     api.ServerSelfUpdateCause("other-cause"),
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t)),

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name: "success - rebooting",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t, withStatus(api.ServerStatusOffline), withStatusDetail(api.ServerStatusDetailOfflineRebooting))),

			assertErr: require.NoError,
			assertLog: log.Empty,
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
					Certificate:   string(serverCertPEM),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
					Channel:       "stable",
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
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
			assertLog: log.Empty,
		},
		{
			name: "error - repo.GetByCertificate",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name: "error - validation",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             ":|//", // invalid URL
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t)),

			assertErr: errassert.ValidationError,
			assertLog: log.Empty,
		},
		{
			name: "error - repo.UpdateByID",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t)),
			repoUpdateErr:              boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name: "error - repo.GetByName",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateServer: ptr.To(validServer(t)),
			repoGetByNameErr:           boom.Error,

			assertErr: require.NoError, // handled async in Goroutine, error is logged.
			assertLog: log.Contains("Failed to update server configuration after self update"),
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
					return &provisioning.Server{}, tc.repoGetByNameErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetByCertificateFunc: func(ctx context.Context, certificatePEM string) (*provisioning.Server, error) {
					return tc.repoGetByCertificateServer, tc.repoGetByCertificateErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithInitialConnectionDelay(1*time.Millisecond),
			)

			// Run test
			err = serverSvc.SelfUpdate(t.Context(), tc.serverSelfUpdate)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
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
			repoGetByName:        validServer(t),

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
			name:                  "error - client.GetResources",
			repoGetAllWithFilter:  provisioning.Servers{},
			repoCreateID:          1,
			repoGetByName:         validServer(t),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
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
			name:                "success",
			oldName:             "one",
			newName:             "one-new",
			repoGetByNameServer: ptr.To(validServer(t, withCluster(nil))),

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
			name:                "error - server is clustered",
			oldName:             "one",
			newName:             "two",
			repoGetByNameServer: ptr.To(validServer(t)),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:                "error - repo.Rename",
			oldName:             "one",
			newName:             "one-new",
			repoGetByNameServer: ptr.To(validServer(t, withCluster(nil))),
			repoRenameErr:       boom.Error,

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

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
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
			name:                "success",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t, withCluster(nil))),

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
			name:                "error - assigned to cluster",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t)),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to delete server, server is part of cluster "one"`)
			},
		},
		{
			name:                "error - repo.DeleteByName",
			nameArg:             "one",
			repoGetByNameServer: ptr.To(validServer(t, withCluster(nil))),
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
				validServer(t, withName("one"), withStatus(api.ServerStatusPending)),
				validServer(t, withName("two")),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err := serverSvc.PollServers(t.Context(), provisioning.ServerFilter{
				Status: ptr.To(api.ServerStatusPending),
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
		assertLog               func(t *testing.T, logBuf *bytes.Buffer)
		assertServerCertificate string
		wantServerStatus        api.ServerStatus
		wantLastSeen            time.Time
	}{
		{
			name:      "success",
			serverArg: validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
				},
			},
			repoUpdate: []queue.Item[struct{}]{
				{},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},

			assertErr:               require.NoError,
			assertLog:               log.Empty,
			assertServerCertificate: string(certPEM),
			wantServerStatus:        api.ServerStatusReady,
			wantLastSeen:            fixedDate,
		},
		{
			name:      "error - client Ping - server state unknown",
			serverArg: validServer(t, withStatus(api.ServerStatusUnknown)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusUnknown))),
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
			name:      "error - client Ping - server state pending",
			serverArg: validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        errassert.RetryableBoomError,
			assertLog:        log.Empty,
			wantServerStatus: api.ServerStatusPending,
		},
		{
			name:      "error - client Ping - server state offline rebooting",
			serverArg: validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusOffline), withStatusDetail(api.ServerStatusDetailOfflineRebooting))),
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr:        errassert.RetryableBoomError,
			assertLog:        log.Empty,
			wantServerStatus: api.ServerStatusOffline,
		},
		{
			name:      "error - client Ping - server state offline shutdown",
			serverArg: validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusOffline), withStatusDetail(api.ServerStatusDetailOfflineShutdown))),
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
			name:      "error - client Ping - server state offline unresponsive",
			serverArg: validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusOffline), withStatusDetail(api.ServerStatusDetailOfflineUnresponsive))),
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
			name:      "error - client Ping with tls.CertificateVerificationError but server is not part of cluster",
			serverArg: validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
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
			assertLog:        log.Empty,
			wantServerStatus: api.ServerStatusPending,
		},
		{
			name:      "success - cluster now has publicly valid certificate",
			serverArg: validServer(t, withClusterCertificate(ptr.To(string(certPEM)))),
			repoGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: ptr.To(validServer(t)),
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
				Name:        "one",
				Certificate: ptr.To(string(certPEM)),
			},

			assertErr:               require.NoError,
			assertLog:               log.Empty,
			assertServerCertificate: string(certPEM),
			wantServerStatus:        api.ServerStatusReady,
			wantLastSeen:            fixedDate,
		},
		{
			name: "error - client Ping with tls.CertificateVerificationError but second ping fails",
			serverArg: provisioning.Server{
				Name:               "one",
				Status:             api.ServerStatusReady,
				Cluster:            ptr.To("cluster"),
				ClusterCertificate: ptr.To("certificate"),
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
				Certificate: ptr.To("certificate"),
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
				Cluster:            ptr.To("cluster"),
				ClusterCertificate: ptr.To("certificate"),
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
			assertLog: log.Empty,
		},
		{
			name: "error - cluster now has publicly valid certificate - clusterSvc.Update",
			serverArg: provisioning.Server{
				Name:               "one",
				Status:             api.ServerStatusReady,
				Cluster:            ptr.To("cluster"),
				ClusterCertificate: ptr.To("certificate"),
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
				Certificate: ptr.To("certificate"),
			},
			clusterSvcUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name: "success - standalone server now has publicly valid certificate",
			serverArg: provisioning.Server{
				Name: "one",
				Certificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
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
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
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
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
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
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
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
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
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
				// Simulate failing connection with pinned certificate, because cluster
				// now has a publicly valid certificate (e.g. ACME).
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
					require.Equal(t, tc.assertServerCertificate, server.Certificate)
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

			serverSvc := provisioningServer.New(repo, client, runner, nil, nil, nil, updateSvc, tls.Certificate{},
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
		repoUpdateErr                  error
		updateSvcGetAllWithFilter      provisioning.Updates
		updateSvcGetAllWithFilterErr   error

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name:                  "success",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
			updateServerConfigArg: true,
			repoGetByName:         ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
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
				UpdateChannel: "stable",
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name:                  "success - without config update",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
			updateServerConfigArg: false,
			repoGetByName:         ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
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
				UpdateChannel: "stable",
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name:                  "success - updating",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
			updateServerConfigArg: true,
			repoGetByName:         ptr.To(validServer(t, withStatusDetail(api.ServerStatusDetailReadyUpdating))),
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
				UpdateChannel: "stable",
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name:                  "success - pending registration",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending), withStatusDetail(api.ServerStatusDetailPendingRegistering)),
			updateServerConfigArg: true,
			repoGetByName:         ptr.To(validServer(t, withStatus(api.ServerStatusPending), withStatusDetail(api.ServerStatusDetailPendingRegistering))),
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
				UpdateChannel: "stable",
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},
		{
			name:                  "success - evacuated",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyEvacuating,
				VersionData: api.ServerVersionData{
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
				Applications: []api.ApplicationVersionData{
					{
						Name:          string(images.UpdateFileComponentIncus),
						Version:       "1",
						InMaintenance: api.InMaintenanceEvacuated,
					},
				},
				UpdateChannel: "stable",
			},
			updateSvcGetAllWithFilter: provisioning.Updates{
				{
					UUID:     uuidgen.FromPattern(t, "1"),
					Version:  "1",
					Channels: []string{"stable"},
					Files: provisioning.UpdateFiles{
						{
							Component: images.UpdateFileComponentOS,
						},
						{
							Component: images.UpdateFileComponentIncus,
						},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.Empty,
		},

		{
			name:                  "error - client IsReady",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
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
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:                  "error - client GetResources",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
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
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:                  "error - client GetOSData",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
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
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:                  "error - server without ip address on management interface",
			serverArg:             validServer(t),
			updateServerConfigArg: true,
			repoGetByName:         ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
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
			assertLog: log.Empty,
		},
		{
			name:                  "error - client GetVersionData",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
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
			assertLog: log.Empty,
		},
		{
			name:                  "error - update channel mismatch",
			serverArg:             validServer(t),
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
			name:                  "error - GetByName",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
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
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:                  "error - pending update with server registration scriptlet error",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending), withStatusDetail(api.ServerStatusDetailPendingRegistering)),
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
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},
			runnerServerRegistrationRunErr: boom.Error,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to run server registration scriptlet: boom!"),
		},
		{
			name:                  "error - enrichServerWithVersionDetails",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
			updateServerConfigArg: true,
			repoGetByName: &provisioning.Server{
				Name:         "one",
				Status:       api.ServerStatusReady,
				StatusDetail: api.ServerStatusDetailReadyUpdating,
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
				UpdateChannel: "stable",
			},
			updateSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
		},
		{
			name:                  "error - Update",
			serverArg:             validServer(t, withStatus(api.ServerStatusPending)),
			repoGetByName:         ptr.To(validServer(t, withStatus(api.ServerStatusPending))),
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
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "stable",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Empty,
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

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.updateSvcGetAllWithFilter, tc.updateSvcGetAllWithFilterErr
				},
			}

			serverSvc := provisioningServer.New(repo, client, runner, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err = serverSvc.PollServer(context.Background(), tc.serverArg, tc.updateServerConfigArg)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
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
			return ptr.To(validServer(t, withStatus(api.ServerStatusPending))), nil
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

	serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
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
		name                  string
		resourceTypeArg       domain.ResourceType
		lifecycleOperationArg domain.LifecycleOperation
		repoGetByName         provisioning.Server
		repoGetByNameErr      error
		repoUpdateErr         error

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
			repoGetByName:         validServer(t),

			assertErr:    require.NoError,
			wantLastSeen: fixedDate,
		},
		{
			name:                  "success - evacuate operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationEvacuate,
			repoGetByName:         validServer(t),

			assertErr: require.NoError,
		},
		{
			name:                  "success - restore operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationRestore,
			repoGetByName:         validServer(t),

			assertErr: require.NoError,
		},
		{
			name:                  "success - evacuate operation - non incus",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationEvacuate,
			repoGetByName:         validServer(t, withName("operations-center"), withType(api.ServerTypeOperationsCenter)),

			assertErr: require.NoError,
		},
		{
			name:                  "success - not supported operation",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperation(""), // empty operation
			repoGetByName:         validServer(t, withName("operations-center"), withType(api.ServerTypeOperationsCenter)),

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
			name:                  "error - pollServer",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationUpdate,
			repoGetByName:         validServer(t),
			repoUpdateErr:         boom.Error,

			assertErr:    boom.ErrorIs,
			wantLastSeen: fixedDate,
		},
		{
			name:                  "error - evacuate operation - repo.Update",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationEvacuate,
			repoGetByName:         validServer(t),
			repoUpdateErr:         boom.Error,

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

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, serverCertificate,
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
								Name:    "os",
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
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
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
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
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
					"os": {
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
					"os": {
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
								Name:    "os",
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
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
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
								Name:    "os",
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
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
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
								Name:    "os",
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
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
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
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
								},
							},
						},
						{
							UUID:     updateV1UUID,
							Version:  "1",
							Channels: []string{"stable"},
							Files: provisioning.UpdateFiles{
								{
									Component: images.UpdateFileComponentOS,
								},
								{
									Component: images.UpdateFileComponentIncus,
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
		repoGetByName                                   provisioning.Server
		repoGetByNameErr                                error
		repoUpdateErrs                                  queue.Errs
		clientEvacuateErr                               error
		clusterSvcIsInstanceLifecycleOperationPermitted bool
		doCallback                                      func(f func(ctx context.Context, err error))
		initVolatileServerState                         func(serverSvc provisioning.ServerService)

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name:          "success - lifecycle operation permitted",
			repoGetByName: validServer(t),
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
			repoGetByName:    validServer(t),
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:          "success - force",
			argForce:      true,
			repoGetByName: validServer(t),
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - operation in flight",
			argClusterUpdate: true,
			repoGetByName:    validServer(t),
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
			repoGetByName:    validServer(t),
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
			name:          "error - callback error",
			repoGetByName: validServer(t),
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to evacuate system name=one err=boom!"),
		},
		{
			name:             "error - cluster update - callback error",
			argClusterUpdate: true,
			repoGetByName:    validServer(t),
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), boom.Error)
			},

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to evacuate system name=one err=boom!"),
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - not type incus",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name:          "error - cluster lifecycle operation not permitted",
			repoGetByName: validServer(t),
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name:          "error - repo.Update",
			repoGetByName: validServer(t),
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Evacuate",
			repoGetByName: validServer(t),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientEvacuateErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Evacuate - reverter error",
			repoGetByName: validServer(t),
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
				EvacuateFunc: func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
					tc.doCallback(callback)
					return tc.clientEvacuateErr
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
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
			name:          "success - lifecycle operation permitted",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:          "success - force",
			argForce:      true,
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),

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
			name:          "error - cluster lifecycle operation not permitted",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name:          "error - repo.Update",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Poweroff",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientPoweroffErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Poweroff and reverter error",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
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
			name:          "success - lifecycle operation permitted",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:          "success - force",
			argForce:      true,
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:          "success - operation in flight",
			argForce:      true,
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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
			name:          "error - cluster lifecycle operation not permitted",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name:          "error - repo.Update",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Reboot",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientRebootErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Reboot and reverter error",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
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
			name:          "success - lifecycle operation permitted",
			repoGetByName: validServer(t),
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
			repoGetByName:    validServer(t),
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:          "success - force",
			argForce:      true,
			repoGetByName: validServer(t),
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: require.NoError,
			assertLog: log.Noop,
		},
		{
			name:             "success - cluster update - operation in flight",
			argClusterUpdate: true,
			repoGetByName:    validServer(t),
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
			repoGetByName:    validServer(t),
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
			name:          "error - callback error",
			repoGetByName: validServer(t),
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
			repoGetByName:    validServer(t),
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
			name:          "error - not type incus",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name:          "error - cluster lifecycle operation not permitted",
			repoGetByName: validServer(t),
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name:          "error - repo.Update",
			repoGetByName: validServer(t),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Restore",
			repoGetByName: validServer(t),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			clientRestoreErr: boom.Error,
			doCallback: func(f func(ctx context.Context, err error)) {
				f(t.Context(), nil)
			},

			assertErr: boom.ErrorIs,
			assertLog: log.Noop,
		},
		{
			name:          "error - client.Restore and reverter error",
			repoGetByName: validServer(t),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, clusterSvc, nil, updateSvc, tls.Certificate{},
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
			name:          "success",
			repoGetByName: validServer(t),

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:          "error - not type incus",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:          "error - repo.Update",
			repoGetByName: validServer(t),
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

			serverSvc := provisioningServer.New(repo, nil, nil, nil, nil, nil, updateSvc, tls.Certificate{},
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

		assertErr require.ErrorAssertionFunc
		assertLog log.MatcherFunc
	}{
		{
			name:          "success - no update triggered",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),

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
			name:          "error - server not ready",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter), withStatus(api.ServerStatusPending)),

			assertErr: errassert.OperationNotPermittedError,
			assertLog: log.Noop,
		},
		{
			name:          "error - cluster lifecycle operation not permitted",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: false,

			assertErr: errassert.OperationNotPermittedErrorContains("Lifecycle operation for server"),
			assertLog: log.Noop,
		},
		{
			name:          "error - repo.Update",
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
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
			repoGetByName: validServer(t, withType(api.ServerTypeOperationsCenter)),
			clusterSvcIsInstanceLifecycleOperationPermitted: true,
			repoUpdateErrs: queue.Errs{
				nil,
				boom.Error,
			},
			clientUpdateOSErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: log.Match("Failed to restore previous server state after failed to update the system server=one err=.*boom!"),
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

			serverSvc := provisioningServer.New(repo, client, nil, nil, clusterSvc, channelSvc, updateSvc, tls.Certificate{},
				provisioningServer.WithWarningEmitter(provisioning.NoopWarningService{}),
			)

			// Run test
			err = serverSvc.UpdateSystemByName(t.Context(), "one", tc.argUpdateRequest, tc.argForce)

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
			name:                           "success - without tokenID and without tokenSeedName",
			argName:                        "one",
			repoGetByName:                  validServer(t, withCluster(nil)),
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},

			assertErr: require.NoError,
		},
		{
			name:                           "success - with tokenID and tokenSeedName",
			argName:                        "one",
			argTokenID:                     ptr.To(uuidgen.FromPattern(t, "1")),
			argTokenSeedName:               ptr.To("some_seed"),
			repoGetByName:                  validServer(t, withCluster(nil)),
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
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByName:    validServer(t, withCluster(nil)),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:          "error - operations center",
			argName:       "one",
			repoGetByName: validServer(t, withCluster(nil), withName("operations-center"), withType(api.ServerTypeOperationsCenter)),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:          "error - incus clustered",
			argName:       "one",
			repoGetByName: validServer(t, withCluster(ptr.To("cluster"))),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:          "error - client.Ping",
			argName:       "one",
			repoGetByName: validServer(t, withCluster(nil)),
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                          "error - tokenSvc.GetTokenSeedByName",
			argName:                       "one",
			argTokenID:                    ptr.To(uuidgen.FromPattern(t, "1")),
			argTokenSeedName:              ptr.To("some_seed"),
			repoGetByName:                 validServer(t, withCluster(nil)),
			tokenSvcGetTokenSeedByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                           "error - tokenSvc.Create",
			argName:                        "one",
			repoGetByName:                  validServer(t, withCluster(nil)),
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			tokenSvcCreateErr:              boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                              "error - tokenSvc.GetTokenProviderConfig",
			argName:                           "one",
			repoGetByName:                     validServer(t, withCluster(nil)),
			tokenSvcGetTokenProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                           "error - client.SystemFactoryReset",
			argName:                        "one",
			repoGetByName:                  validServer(t, withCluster(nil)),
			tokenSvcGetTokenProviderConfig: &api.TokenProviderConfig{},
			clientSystemFactoryResetErr:    boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                           "error - repo.DeleteByName",
			argName:                        "one",
			repoGetByName:                  validServer(t, withCluster(nil)),
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
			err := serverSvc.FactoryResetByName(t.Context(), tc.argName, tc.argTokenID, tc.argTokenSeedName)

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
			name:          "success",
			argName:       "one",
			repoGetByName: ptr.To(validServer(t)),
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
			name:                      "error - client.GetSystemLogging",
			argName:                   "one",
			repoGetByName:             ptr.To(validServer(t)),
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
			repoGetByName:    ptr.To(validServer(t)),

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                         "error - client.UpdateSystemLogging",
			argName:                      "one",
			repoGetByName:                ptr.To(validServer(t)),
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
		repoGetByNameErr         error
		clientGetSystemKernel    provisioning.ServerSystemKernel
		clientGetSystemKernelErr error

		assertErr        require.ErrorAssertionFunc
		wantKernelConfig provisioning.ServerSystemKernel
	}{
		{
			name: "success",
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
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                     "error - client.GetSystemKernel",
			clientGetSystemKernelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return ptr.To(validServer(t)), tc.repoGetByNameErr
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
			kernelConfig, err := serverSvc.GetSystemKernel(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantKernelConfig, kernelConfig)
		})
	}
}

func TestServerService_UpdateSystemKernel(t *testing.T) {
	tests := []struct {
		name                        string
		argKernelConfig             incusosapi.SystemKernel
		repoGetByNameErr            error
		clientUpdateSystemKernelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "success",
			argKernelConfig: incusosapi.SystemKernel{},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                        "error - client.UpdateSystemKernel",
			clientUpdateSystemKernelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return ptr.To(validServer(t)), tc.repoGetByNameErr
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
			err := serverSvc.UpdateSystemKernel(t.Context(), "one", tc.argKernelConfig)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_AddApplication(t *testing.T) {
	tests := []struct {
		name                    string
		argApplicationName      string
		repoGetByNameErr        error
		clientAddApplicationErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success",
			argApplicationName: "debug",

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - client.AddApplication",
			clientAddApplicationErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return ptr.To(validServer(t)), tc.repoGetByNameErr
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
			err := serverSvc.AddApplication(t.Context(), "one", tc.argApplicationName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_SyncCluster(t *testing.T) {
	s := provisioningServer.New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{})
	err := s.SyncCluster(t.Context(), "")
	require.NoError(t, err)
}
