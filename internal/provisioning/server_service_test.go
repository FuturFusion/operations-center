package provisioning_test

import (
	"bytes"
	"context"
	"crypto/tls"
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
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerService_UpdateServerURL(t *testing.T) {
	serverCertPEM, serverKeyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	serverCertificate, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                    string
		argServerURL            string
		repoGetAllWithFilter    provisioning.Servers
		repoGetAllWithFilterErr error
		repoGetByName           provisioning.Server
		repoUpdateErr           error
		repoCreateErr           error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:         "success - operations center self update",
			argServerURL: "https://new:8443",
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   string(serverCertPEM),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                 "success - operations center self update - no server of type operations center - trigger self register",
			argServerURL:         "https://new:8443",
			repoGetAllWithFilter: provisioning.Servers{},
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:                    "error - operations center self update - repo.GetAllWithFilter",
			argServerURL:            "https://new:8443",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:         "error - operations center self update - multiple servers of type operations center",
			argServerURL: "https://new:8443",
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
		{
			name:         "error - operations center self update - validation",
			argServerURL: ":|//", // invalid URL
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   string(serverCertPEM),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:         "error - operations center self update - repo.Update",
			argServerURL: "https://new:8443",
			repoGetAllWithFilter: provisioning.Servers{
				{
					Name:          "one",
					ConnectionURL: "http://one/",
					Certificate:   string(serverCertPEM),
					Type:          api.ServerTypeOperationsCenter,
					Status:        api.ServerStatusReady,
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
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "https://one:8443", serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.UpdateServerURL(t.Context(), tc.argServerURL)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_UpdateCertificate(t *testing.T) {
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
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "https://one:8443", serverCertificate,
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
				Status: api.ServerStatusReady,
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
				Type:   api.ServerTypeOperationsCenter,
				Status: api.ServerStatusReady,
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
				Status: api.ServerStatusReady,
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
				Status: api.ServerStatusReady,
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
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				ConsumeFunc: func(ctx context.Context, id uuid.UUID) error {
					return tc.tokenSvcConsumeErr
				},
			}

			token := uuid.MustParse("686d2a12-20f9-11f0-82c6-7fff26bab0c4")

			serverSvc := provisioning.NewServerService(repo, client, tokenSvc, nil, "", tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithInitialConnectionDelay(0), // Disable delay for initial connection test
			)

			// Run test
			_, err := serverSvc.Create(context.Background(), token, tc.server)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{})

			// Run test
			servers, err := serverSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, servers, tc.count)
		})
	}
}

func TestServerService_GetAllWithFilter(t *testing.T) {
	tests := []struct {
		name                    string
		filter                  provisioning.ServerFilter
		repoGetAllWithFilter    provisioning.Servers
		repoGetAllWithFilterErr error

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
			name:                    "error - repo",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{})

			// Run test
			server, err := serverSvc.GetAllWithFilter(context.Background(), tc.filter)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{})

			// Run test
			serverNames, err := serverSvc.GetAllNames(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverNames, tc.count)
		})
	}
}

func TestServerService_GetAllIDsWithFilter(t *testing.T) {
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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{})

			// Run test
			serverIDs, err := serverSvc.GetAllNamesWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverIDs, tc.count)
		})
	}
}

func TestServerService_GetByName(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
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
			name:             "error - repo",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{})

			// Run test
			server, err := serverSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByNameServer, server)
		})
	}
}

func TestServerService_Update(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name          string
		server        provisioning.Server
		repoUpdateErr error

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
				Status: api.ServerStatusReady,
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
				Status: api.ServerStatusReady,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
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
				Status: api.ServerStatusReady,
			},
			repoUpdateErr: boom.Error,

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
			}

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.Update(context.Background(), tc.server)

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
			ctx:  context.Background(),
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
			ctx:              context.Background(),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.UpdateByID",
			ctx:  context.Background(),
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
				ctx, cancel := context.WithCancelCause(context.Background())
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
				Status: api.ServerStatusReady,
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
			ctx:  context.Background(),
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
			ctx:  context.Background(),
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

			// Register our own self update signal, such that we can ensure, that all the listeners
			// have been removed after successful processing.
			selfUpdateSignal := signals.New[provisioning.Server]()

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "", tls.Certificate{},
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "", tls.Certificate{})

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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "", tls.Certificate{})

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
				Status: api.ServerStatusReady,
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

			selfUpdateSignal := signals.New[provisioning.Server]()

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "", tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithSelfUpdateSignal(selfUpdateSignal),
			)

			// Run test
			wg := sync.WaitGroup{}
			wg.Add(1)

			var err error
			go func() {
				defer wg.Done()

				err = serverSvc.UpdateSystemNetwork(context.Background(), "one", provisioning.ServerSystemNetwork{})
			}()

			// Wait for subscriber.
			for selfUpdateSignal.IsEmpty() {
				time.Sleep(time.Millisecond)
			}

			// Simulate update from a different node, which is ignored.
			selfUpdateSignal.Emit(context.Background(), provisioning.Server{
				Name: "another",
			})

			selfUpdateSignal.Emit(context.Background(), provisioning.Server{
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
				GetByCertificateFunc: func(ctx context.Context, certificatePEM string) (*provisioning.Server, error) {
					return tc.repoGetByCertificateServer, tc.repoGetByCertificateErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					require.Equal(t, fixedDate, in.LastSeen)
					return tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "https://one:8443", serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.SelfUpdate(context.Background(), tc.serverSelfUpdate)

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
		argServerURL            string
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
			argServerURL:         "https://192.168.1.200:8443",
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateID:         1,
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:                    "error - repo.GetAllWithFilter",
			argServerURL:            "https://192.168.1.200:8443",
			repoGetAllWithFilterErr: boom.Error,
			repoCreateID:            1,

			assertErr: boom.ErrorIs,
		},
		{
			name:         "error - Operations Center is already registered",
			argServerURL: "https://192.168.1.200:8443",
			repoGetAllWithFilter: provisioning.Servers{
				{},
			},
			repoCreateID: 1,

			assertErr: require.Error,
		},
		{
			name:                 "error - validation",
			argServerURL:         ":|//", // invalid URL
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateID:         1,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:                 "error - repo.Create",
			argServerURL:         "https://192.168.1.200:8443",
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateErr:        boom.Error,
			repoCreateID:         1,

			assertErr: boom.ErrorIs,
		},
		{
			name:                 "success - Operations Center initial self update (registration)",
			argServerURL:         "https://192.168.1.200:8443",
			repoGetAllWithFilter: provisioning.Servers{},
			repoCreateID:         1,
			repoGetByName: provisioning.Server{
				Name:   "operations-center",
				Status: api.ServerStatusReady,
			},
			clientGetResourcesErr: boom.Error,

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
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, tc.argServerURL, serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.SelfRegisterOperationsCenter(context.Background())

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_Rename(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name          string
		oldName       string
		newName       string
		repoRenameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			oldName: "one",
			newName: "one-new",

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
			name:          "error - repo.GetByName",
			oldName:       "one",
			newName:       "one-new",
			repoRenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				RenameFunc: func(ctx context.Context, oldName string, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.Rename(context.Background(), tc.oldName, tc.newName)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, nil, "", tls.Certificate{})

			// Run test
			err := serverSvc.DeleteByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_PollPendingServers(t *testing.T) {
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
		repoGetByNameServer         provisioning.Server
		repoGetByNameErr            error
		repoUpdateErr               error
		clusterSvcGetByName         *provisioning.Cluster
		clusterSvcGetByNameErr      error
		clusterSvcUpdateErr         error

		assertErr require.ErrorAssertionFunc
		assertLog func(t *testing.T, logBuf *bytes.Buffer)
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
			},
			clientPing: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			assertLog: logEmpty,
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
				Certificate: "certificate",
			},

			assertErr: require.NoError,
			assertLog: logEmpty,
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
				Certificate: "certificate",
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
				Certificate: "certificate",
			},
			clusterSvcUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
			assertLog: logEmpty,
		},
		{
			name: "success - standalone server now has publicly valid certificate",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:                "ready",
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

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "", tls.Certificate{},
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
				provisioning.ServerServiceWithHTTPClient(httpsServer.Client()),
			)
			serverSvc.SetClusterService(clusterSvc)

			// Run test
			err = serverSvc.PollServers(context.Background(), api.ServerStatusPending, true)

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
		name             string
		repoGetByName    provisioning.Server
		repoGetByNameErr error
		repoUpdateErr    error

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
			name:          "error - pollServer",
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
				GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
					return api.ServerTypeIncus, nil
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, nil, "https://one:8443", serverCertificate,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.ResyncByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}
