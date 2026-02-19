package provisioning_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid internal state, expect at most 1 server of type "operations-center", found 2`)
			},
		},
		// validateion error not covered
		{
			name:           "error - operations center self update - repo.Update",
			argCertificate: serverCertificate,
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
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, nil
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{}, nil
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return api.ServerVersionData{}, nil
				},
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, nil, serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
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
			name: "success",
			server: provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:               "error - token consume",
			tokenSvcConsumeErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - remote Operations Center",
			server: provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Type:    api.ServerTypeOperationsCenter,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			server: provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - Ping",
			server: provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
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
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, nil
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{}, nil
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

			token := uuid.MustParse("686d2a12-20f9-11f0-82c6-7fff26bab0c4")

			serverSvc := provisioning.NewServerService(repo, client, tokenSvc, nil, nil, nil, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithInitialConnectionDelay(0), // Disable delay for initial connection test
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
				provisioning.Server{
					Name:          "one",
					Cluster:       ptr.To("one"),
					ConnectionURL: "http://one/",
				},
				provisioning.Server{
					Name:          "two",
					Cluster:       ptr.To("one"),
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, updateSvc, tls.Certificate{})

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
				Expression: ptr.To(`name == "one"`),
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
			name: "error - invalid filter expression",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(``), // the empty expression is an invalid expression.
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to compile filter expression:")
			},
			count: 0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to execute filter expression:")
			},
			count: 0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllWithFilter: provisioning.Servers{
				provisioning.Server{
					Name: "one",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
			count: 0,
		},
		{
			name: "error - upodateSvc.GetAllWithFilter",
			filter: provisioning.ServerFilter{
				Cluster: ptr.To("one"),
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, updateSvc, tls.Certificate{})

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, nil, tls.Certificate{})

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
			name: "error - invalid filter expression",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(``), // the empty expression is an invalid expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to compile filter expression:")
			},
			count: 0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to execute filter expression:")
			},
			count: 0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
			count: 0,
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, nil, tls.Certificate{})

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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
			wantServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					NeedsUpdate:   ptr.To(false),
					NeedsReboot:   ptr.To(false),
					InMaintenance: ptr.To(false),
					OS: api.OSVersionData{
						NeedsUpdate: ptr.To(false),
					},
				},
			},
		},
		{
			name:    "success - with version data and updates - everything up to date",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "os",
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
				ConnectionURL: "http://one/",
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
					InMaintenance: ptr.To(false),
				},
			},
		},
		{
			name:    "success - with version data and updates - update available",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "os",
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
				ConnectionURL: "http://one/",
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
					InMaintenance: ptr.To(false),
				},
			},
		},
		{
			name:    "success - with version data and updates - no update information for incus-ceph",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						Name:        "os",
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
				ConnectionURL: "http://one/",
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
					InMaintenance: ptr.To(false),
				},
			},
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
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
				Cluster:       ptr.To("one"),
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, updateSvc, tls.Certificate{})

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
		name                string
		argForce            bool
		server              provisioning.Server
		repoUpdateErr       error
		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByNameServer: &provisioning.Server{
				Name:    "one",
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:     "error - repo.GetByName - without force",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - channel update for clustered server",
			argForce: false,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByNameServer: &provisioning.Server{
				Name:    "one",
				Cluster: ptr.To("one"),
				Channel: "testing",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Update of channel not allowed for clustered server "one"`)
			},
		},
		{
			name: "error - repo.UpdateByID",
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByNameServer: &provisioning.Server{
				Name:    "one",
				Channel: "stable",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - repo.GetByName - force", // UpdateSystemUpdate
			argForce: true,
			server: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					return tc.repoUpdateErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return errors.New("") // short cirquite pollServer, since we don't care about this part in this test.
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, channelSvc, updateSvc, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.Update(t.Context(), tc.server, tc.argForce, true)

			// Assert
			tc.assertErr(t, err)
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithSelfUpdateSignal(selfUpdateSignal),
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

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
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, channelSvc, updateSvc, tls.Certificate{})

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
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
				Certificate: `-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`,
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithSelfUpdateSignal(selfUpdateSignal),
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
				Certificate:   string(serverCertPEM),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr: require.NoError,
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
		},
		{
			name: "error - repo.GetByCertificate",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: serverCertificate.Leaf,
			},
			repoGetByCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
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
				Certificate:   string(serverCertPEM),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
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
				Certificate:   string(serverCertPEM),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
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
				Certificate:   string(serverCertPEM),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
				Channel:       "stable",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
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
					require.Equal(t, fixedDate, in.LastSeen)
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, nil
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{}, nil
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return api.ServerVersionData{}, nil
				},
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, nil, serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.SelfUpdate(t.Context(), tc.serverSelfUpdate)

			// Assert
			tc.assertErr(t, err)
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
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, tc.clientGetResourcesErr
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{}, nil
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return api.ServerVersionData{}, nil
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

			err = config.UpdateNetwork(t.Context(), api.SystemNetworkPut{
				OperationsCenterAddress: "https://192.168.1.200:8443",
				RestServerAddress:       "[::]:8443",
			})
			require.NoError(t, err)

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - new name empty",
			oldName: "one",
			newName: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - old and new name equal",
			oldName: "one",
			newName: "one", // equal

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
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
				Cluster: ptr.To("one"), // server already clustered
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
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
				Cluster: ptr.To("one"),
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			err := serverSvc.DeleteByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_PollServers(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	logEmpty := func(t *testing.T, logBuf *bytes.Buffer) {
		t.Helper()

		require.Empty(t, logBuf.String())
	}

	logMatch := func(expr string) func(t *testing.T, logBuf *bytes.Buffer) {
		re, err := regexp.Compile(expr)
		require.NoError(t, err)

		return func(t *testing.T, logBuf *bytes.Buffer) {
			t.Helper()

			// Give logs a little bit of time to be processed.
			for range 5 {
				if re.Match(logBuf.Bytes()) {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			require.True(t, re.Match(logBuf.Bytes()), "logBuf did not match expression: %q, logBuf:\n%s", expr, logBuf.String())
		}
	}

	httpsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	httpsServer.StartTLS()
	defer httpsServer.Close()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name                        string
		repoGetAllWithFilterServers provisioning.Servers
		repoGetAllWithFilterErr     error
		clientPing                  []queue.Item[struct{}]
		clientGetResourcesErr       error
		clientGetOSDataErr          error
		clientGetVersionData        api.ServerVersionData
		clientGetVersionDataErr     error
		repoGetByNameServer         provisioning.Server
		repoGetByNameErr            error
		repoUpdateErr               error
		clusterSvcGetByName         *provisioning.Cluster
		clusterSvcGetByNameErr      error
		clusterSvcUpdateErr         error

		assertErr               require.ErrorAssertionFunc
		assertLog               func(t *testing.T, logBuf *bytes.Buffer)
		assertServerCertificate string
	}{
		{
			name:                        "success - no pending servers",
			repoGetAllWithFilterServers: provisioning.Servers{},

			assertErr: require.NoError,
			assertLog: logEmpty,
		},
		{
			name: "success",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			repoGetByNameServer: provisioning.Server{
				Name:   "pending",
				Status: api.ServerStatusPending,
				Certificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			assertLog: logEmpty,
			assertServerCertificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
		},
		{
			name:                    "error - GetAllWithFilter",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "error - client Ping",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog: logMatch("Server connection test failed"),
		},
		{
			name: "error - client Ping with tls.CertificateVerificationError but server is not part of cluster",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPing: []queue.Item[struct{}]{
				{
					Err: &url.Error{
						Err: &tls.CertificateVerificationError{},
					},
				},
			},

			assertErr: require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog: logMatch("Server connection test failed"),
		},
		{
			name: "success - cluster now has publicly valid certificate",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "ready",
					Status: api.ServerStatusReady,
					Certificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
					Cluster:            ptr.To("cluster"),
					ClusterCertificate: ptr.To("certificate"),
				},
			},
			repoGetByNameServer: provisioning.Server{
				Name: "ready",
				Certificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
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

			assertErr: require.NoError,
			assertLog: logEmpty,
			assertServerCertificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
		},
		{
			name: "error - client Ping with tls.CertificateVerificationError but second ping fails",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:               "ready",
					Status:             api.ServerStatusReady,
					Cluster:            ptr.To("cluster"),
					ClusterCertificate: ptr.To("certificate"),
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
				{
					Err: boom.Error,
				},
			},
			clusterSvcGetByName: &provisioning.Cluster{
				Name:        "cluster",
				Certificate: ptr.To("certificate"),
			},

			assertErr: require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
			assertLog: logMatch("Server connection test failed"),
		},
		{
			name: "error - cluster now has publicly valid certificate - clusterSvc.GetByName",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:               "ready",
					Status:             api.ServerStatusReady,
					Cluster:            ptr.To("cluster"),
					ClusterCertificate: ptr.To("certificate"),
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
			clusterSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "error - cluster now has publicly valid certificate - clusterSvc.Update",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:               "ready",
					Status:             api.ServerStatusReady,
					Cluster:            ptr.To("cluster"),
					ClusterCertificate: ptr.To("certificate"),
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
			clusterSvcGetByName: &provisioning.Cluster{
				Name:        "cluster",
				Certificate: ptr.To("certificate"),
			},
			clusterSvcUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "success - standalone server now has publicly valid certificate",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name: "ready",
					Certificate: `-----BEGIN CERTIFICATE-----
foobar
-----END CERTIFICATE-----`,
					Status:              api.ServerStatusReady,
					Type:                api.ServerTypeMigrationManager,
					ConnectionURL:       "https:/127.0.0.1:7443",
					PublicConnectionURL: httpsServer.URL,
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

			assertErr: require.NoError,
			assertLog: logEmpty,
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
		},
		{
			name: "error - standalone server - invalid public connection URL",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:                "ready",
					Status:              api.ServerStatusReady,
					Type:                api.ServerTypeMigrationManager,
					ConnectionURL:       "https:/127.0.0.1:7443",
					PublicConnectionURL: ":|\\", // invalid
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

			assertErr: require.NoError,
			assertLog: logMatch("Server connection test failed"),
		},
		{
			name: "error - standalone server - connection error",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:                "ready",
					Status:              api.ServerStatusReady,
					Type:                api.ServerTypeMigrationManager,
					ConnectionURL:       "https:/127.0.0.1:7443",
					PublicConnectionURL: "https:/127.0.0.1:7443",
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

			assertErr: require.NoError,
			assertLog: logMatch("(?ms)Refresh certificate connection attempt to public connection URL failed.*Server connection test failed"),
		},
		{
			name: "error - standalone server - connection error",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:                "ready",
					Status:              api.ServerStatusReady,
					Type:                api.ServerTypeMigrationManager,
					ConnectionURL:       "https:/127.0.0.1:7443",
					PublicConnectionURL: httpServer.URL,
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

			assertErr: require.NoError,
			assertLog: logMatch("(?ms)Refresh certificate connection attempt did not return TLS connection or no peer certificates.*Server connection test failed"),
		},
		{
			name: "error - client GetResources",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},
			clientGetResourcesErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "error - client GetOSData",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},
			clientGetOSDataErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "error - client GetVersionData",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},
			clientGetVersionDataErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "error - update channel mismatch",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:    "pending",
					Status:  api.ServerStatusPending,
					Channel: "stable",
				},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},
			clientGetVersionData: api.ServerVersionData{
				UpdateChannel: "testing", // does not match expected channel
			},

			assertErr: require.NoError,
			assertLog: logMatch("update channel reported by server does not match expected update channel"),
		},
		{
			name: "error - GetByName",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true)
			require.NoError(t, err)

			repo := &repoMock.ServerRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilterServers, tc.repoGetAllWithFilterErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					require.Equal(t, api.ServerStatusReady, server.Status)
					require.Equal(t, fixedDate, server.LastSeen)
					require.Equal(t, tc.assertServerCertificate, server.Certificate)
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					_, err := queue.Pop(t, &tc.clientPing)
					return err
				},
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, tc.clientGetResourcesErr
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{}, tc.clientGetOSDataErr
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return tc.clientGetVersionData, tc.clientGetVersionDataErr
				},
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			clusterSvc := &svcMock.ClusterServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.clusterSvcGetByName, tc.clusterSvcGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					return tc.clusterSvcUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, nil, tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithHTTPClient(httpsServer.Client()),
			)
			serverSvc.SetClusterService(clusterSvc)

			// Run test
			err = serverSvc.PollServers(t.Context(), api.ServerStatusPending, true)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.clientPing)
		})
	}
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
							Name: string(images.UpdateFileComponentIncus),
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
							Name: string(images.UpdateFileComponentIncus),
						},
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
			name:                  "error - pollServer",
			resourceTypeArg:       domain.ResourceTypeServer,
			lifecycleOperationArg: domain.LifecycleOperationUpdate,
			repoUpdateErr:         boom.Error,

			assertErr:    boom.ErrorIs,
			wantLastSeen: fixedDate,
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
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, nil
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return api.OSData{}, nil
				},
				GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
					return api.ServerVersionData{}, nil
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
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

func TestServerService_EvacuateSystemByName(t *testing.T) {
	tests := []struct {
		name              string
		repoGetByName     provisioning.Server
		repoGetByNameErr  error
		repoUpdateErr     error
		clientEvacuateErr error

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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
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
		{
			name: "error - client.Evacuate",
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
			clientEvacuateErr: boom.Error,

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
					server.VersionData.Compute(nil)
					require.True(t, *server.VersionData.InMaintenance)
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				EvacuateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientEvacuateErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.EvacuateSystemByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_PoweroffSystemByName(t *testing.T) {
	tests := []struct {
		name              string
		repoGetByName     provisioning.Server
		repoGetByNameErr  error
		clientPoweroffErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
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
			name:              "error - client.Poweroff",
			clientPoweroffErr: boom.Error,

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
			}

			client := &adapterMock.ServerClientPortMock{
				PoweroffFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientPoweroffErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.PoweroffSystemByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_RebootSystemByName(t *testing.T) {
	tests := []struct {
		name             string
		repoGetByName    provisioning.Server
		repoGetByNameErr error
		clientRebootErr  error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
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
			name:            "error - client.Reboot",
			clientRebootErr: boom.Error,

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
			}

			client := &adapterMock.ServerClientPortMock{
				RebootFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientRebootErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.RebootSystemByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_RestoreSystemByName(t *testing.T) {
	tests := []struct {
		name             string
		repoGetByName    provisioning.Server
		repoGetByNameErr error
		clientRestoreErr error

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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
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
			clientRestoreErr: boom.Error,

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
			}

			client := &adapterMock.ServerClientPortMock{
				RestoreFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientRestoreErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.RestoreSystemByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_UpdateSystemByName(t *testing.T) {
	tests := []struct {
		name                   string
		updateRequestArg       api.ServerUpdatePost
		repoGetByName          provisioning.Server
		repoGetByNameErr       error
		clientUpdateOSErr      error
		channelSvcGetByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - no update triggered",
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name: "success - trigger OS update",
			updateRequestArg: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
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
			name: "error - UpdateSystemUpdate - channelSvc.GetByName",
			updateRequestArg: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			channelSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateOS",
			updateRequestArg: api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			},
			clientUpdateOSErr: boom.Error,

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
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateOSFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientUpdateOSErr
				},
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return errors.New("") // short cirquite pollServer, since we don't care about this part in this test.
				},
				UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
					return nil
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, channelSvc, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemByName(t.Context(), "one", tc.updateRequestArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_SyncCluster(t *testing.T) {
	s := provisioning.NewServerService(nil, nil, nil, nil, nil, nil, tls.Certificate{})
	err := s.SyncCluster(t.Context(), "")
	require.NoError(t, err)
}
