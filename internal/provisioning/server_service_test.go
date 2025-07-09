package provisioning_test

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

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
				PingFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
				GetResourcesFunc: func(ctx context.Context, server provisioning.Server) (api.HardwareData, error) {
					return api.HardwareData{}, nil
				},
				GetOSDataFunc: func(ctx context.Context, server provisioning.Server) (api.OSData, error) {
					return api.OSData{}, nil
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				ConsumeFunc: func(ctx context.Context, id uuid.UUID) error {
					return tc.tokenSvcConsumeErr
				},
			}

			token := uuid.MustParse("686d2a12-20f9-11f0-82c6-7fff26bab0c4")

			serverSvc := provisioning.NewServerService(repo, client, tokenSvc,
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

			serverSvc := provisioning.NewServerService(repo, nil, nil)

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
				Expression: ptr.To(`Name == "one"`),
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

			assertErr: require.Error,
			count:     0,
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

			assertErr: require.Error,
			count:     0,
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

			assertErr: func(tt require.TestingT, err error, i ...any) {
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

			serverSvc := provisioning.NewServerService(repo, nil, nil)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil)

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
				Expression: ptr.To(`Name matches "one"`),
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

			assertErr: require.Error,
			count:     0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: require.Error,
			count:     0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ServerFilter{
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
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

			serverSvc := provisioning.NewServerService(repo, nil, nil)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

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
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.UpdateByID",
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
			name: "error - client.UpdateNetworkConfig",
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
					return tc.clientUpdateNetworkConfigErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			err := serverSvc.UpdateSystemNetwork(context.Background(), "name", provisioning.ServerSystemNetwork{})

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
		})
	}
}

func TestServerService_SelfUpdate(t *testing.T) {
	// Setup client certificate
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                       string
		serverSelfUpdate           provisioning.ServerSelfUpdate
		repoGetByCertificateServer *provisioning.Server
		repoGetByCertificateErr    error
		repoUpdateErr              error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one-new/",
				AuthenticationCertificate: cert.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   string(certPEM),
				Type:          api.ServerTypeIncus,
				Status:        api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name: "error - repo.GetByCertificate not found",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: cert.Leaf,
			},
			repoGetByCertificateErr: domain.ErrNotFound,

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthorized)
			},
		},
		{
			name: "error - repo.GetByCertificate",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             "http://one/",
				AuthenticationCertificate: cert.Leaf,
			},
			repoGetByCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - validation",
			serverSelfUpdate: provisioning.ServerSelfUpdate{
				ConnectionURL:             ":|//", // invalid URL
				AuthenticationCertificate: cert.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   string(certPEM),
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
				AuthenticationCertificate: cert.Leaf,
			},
			repoGetByCertificateServer: &provisioning.Server{
				Name:          "one",
				ConnectionURL: "http://one/",
				Certificate:   string(certPEM),
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
				GetByCertificateFunc: func(ctx context.Context, certificatePEM string) (*provisioning.Server, error) {
					return tc.repoGetByCertificateServer, tc.repoGetByCertificateErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					require.Equal(t, fixedDate, in.LastSeen)
					return tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, nil, nil, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			err := serverSvc.SelfUpdate(context.Background(), tc.serverSelfUpdate)

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

			serverSvc := provisioning.NewServerService(repo, nil, nil, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

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

			assertErr: func(tt require.TestingT, err error, i ...any) {
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

			serverSvc := provisioning.NewServerService(repo, nil, nil)

			// Run test
			err := serverSvc.DeleteByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_PollPendingServers(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                        string
		repoGetAllWithFilterServers provisioning.Servers
		repoGetAllWithFilterErr     error
		clientPingErr               error
		clientGetResourcesErr       error
		clientGetOSDataErr          error
		repoGetByNameServer         provisioning.Server
		repoGetByNameErr            error
		repoUpdateErr               error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                        "success - no pending servers",
			repoGetAllWithFilterServers: provisioning.Servers{},

			assertErr: require.NoError,
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
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientPingErr: boom.Error,

			assertErr: require.NoError, // Failing of ping is expected and not reported as error but only logged as warning.
		},
		{
			name: "error - client GetResources",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientGetResourcesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client GetOSData",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			clientGetOSDataErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - GetByName",
			repoGetAllWithFilterServers: provisioning.Servers{
				provisioning.Server{
					Name:   "pending",
					Status: api.ServerStatusPending,
				},
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
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
				PingFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientPingErr
				},
				GetResourcesFunc: func(ctx context.Context, server provisioning.Server) (api.HardwareData, error) {
					return api.HardwareData{}, tc.clientGetResourcesErr
				},
				GetOSDataFunc: func(ctx context.Context, server provisioning.Server) (api.OSData, error) {
					return api.OSData{}, tc.clientGetOSDataErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, client, nil,
				provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.PollServers(context.Background(), api.ServerStatusPending, true)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
