package provisioning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestServerService_Create(t *testing.T) {
	tests := []struct {
		name          string
		server        provisioning.Server
		repoCreateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			server: provisioning.Server{
				Hostname:  "one",
				ClusterID: 1,
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Hostname:  "", // invalid
				ClusterID: 1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			server: provisioning.Server{
				Hostname:  "one",
				ClusterID: 1,
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
					return provisioning.Server{}, tc.repoCreateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			_, err := serverSvc.Create(context.Background(), tc.server)

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
					ID:        1,
					Hostname:  "one",
					ClusterID: 1,
				},
				provisioning.Server{
					ID:        2,
					Hostname:  "one",
					ClusterID: 1,
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
			repo := &mock.ServerRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.repoGetAllServers, tc.repoGetAllErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			servers, err := serverSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, servers, tc.count)
		})
	}
}

func TestServerService_GetAllHostnames(t *testing.T) {
	tests := []struct {
		name             string
		repoGetAllIDs    []string
		repoGetAllIDsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllIDs: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:             "error - repo",
			repoGetAllIDsErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetAllHostnamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllIDs, tc.repoGetAllIDsErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			serverHostnames, err := serverSvc.GetAllHostnames(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverHostnames, tc.count)
		})
	}
}

func TestServerService_GetByID(t *testing.T) {
	tests := []struct {
		name              string
		idArg             int
		repoGetByIDServer provisioning.Server
		repoGetByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: 1,
			repoGetByIDServer: provisioning.Server{
				Hostname:  "one",
				ClusterID: 1,
			},

			assertErr: require.NoError,
		},
		{
			name:           "error - repo",
			idArg:          1,
			repoGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByIDFunc: func(ctx context.Context, id int) (provisioning.Server, error) {
					return tc.repoGetByIDServer, tc.repoGetByIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.GetByID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDServer, server)
		})
	}
}

func TestServerService_GetByHostname(t *testing.T) {
	tests := []struct {
		name              string
		hostnameArg       string
		repoGetByIDServer provisioning.Server
		repoGetByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:        "success",
			hostnameArg: "one",
			repoGetByIDServer: provisioning.Server{
				Hostname:  "one",
				ClusterID: 1,
			},

			assertErr: require.NoError,
		},
		{
			name:        "error - hostname empty",
			hostnameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:           "error - repo",
			hostnameArg:    "one",
			repoGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByHostnameFunc: func(ctx context.Context, hostname string) (provisioning.Server, error) {
					return tc.repoGetByIDServer, tc.repoGetByIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.GetByHostname(context.Background(), tc.hostnameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDServer, server)
		})
	}
}

func TestServerService_UpdateByHostname(t *testing.T) {
	tests := []struct {
		name                    string
		hostnameArg             string
		server                  provisioning.Server
		repoGetByHostnameServer provisioning.Server
		repoGetByHostnameErr    error
		repoUpdateServer        provisioning.Server
		repoUpdateErr           error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:        "success",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoGetByHostnameServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoUpdateServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},

			assertErr: require.NoError,
		},
		{
			name:        "error - empty hostname",
			hostnameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:        "error - validation",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "", // invalid
				ClusterID: 1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:        "error - hostname mismatch",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one 1", // mismatch
				ClusterID: 1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:        "error - repo.GetByHostname",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoGetByHostnameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:        "error - repo.UpdateByID",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoGetByHostnameServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByHostnameFunc: func(ctx context.Context, hostname string) (provisioning.Server, error) {
					return tc.repoGetByHostnameServer, tc.repoGetByHostnameErr
				},
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
					return tc.repoUpdateServer, tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.UpdateByHostname(context.Background(), tc.hostnameArg, tc.server)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateServer, server)
		})
	}
}

func TestServerService_RenameByHostname(t *testing.T) {
	tests := []struct {
		name                    string
		hostnameArg             string
		server                  provisioning.Server
		repoGetByHostnameServer provisioning.Server
		repoGetByHostnameErr    error
		repoUpdateServer        provisioning.Server
		repoUpdateErr           error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:        "success",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoGetByHostnameServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoUpdateServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},

			assertErr: require.NoError,
		},
		{
			name:        "error - empty hostname",
			hostnameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:        "error - new hostname empty",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "", // invalid
				ClusterID: 1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:        "error - repo.GetByHostname",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoGetByHostnameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:        "error - repo.UpdateByID",
			hostnameArg: "one",
			server: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoGetByHostnameServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByHostnameFunc: func(ctx context.Context, hostname string) (provisioning.Server, error) {
					return tc.repoGetByHostnameServer, tc.repoGetByHostnameErr
				},
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
					require.Equal(t, tc.server.Hostname, in.Hostname)
					return tc.repoUpdateServer, tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.RenameByHostname(context.Background(), tc.hostnameArg, tc.server)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateServer, server)
		})
	}
}

func TestServerService_DeleteByHostname(t *testing.T) {
	tests := []struct {
		name                    string
		hostnameArg             string
		repoGetByHostnameServer provisioning.Server
		repoGetByHostnameErr    error
		repoDeleteByIDErr       error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:        "success",
			hostnameArg: "one",
			repoGetByHostnameServer: provisioning.Server{
				ID:        1,
				Hostname:  "one",
				ClusterID: 1,
			},

			assertErr: require.NoError,
		},
		{
			name:        "error - hostname empty",
			hostnameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:                 "error - repo.GetByHostname",
			hostnameArg:          "one",
			repoGetByHostnameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:              "error - repo.DeleteByID",
			hostnameArg:       "one",
			repoDeleteByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByHostnameFunc: func(ctx context.Context, hostname string) (provisioning.Server, error) {
					return tc.repoGetByHostnameServer, tc.repoGetByHostnameErr
				},
				DeleteByIDFunc: func(ctx context.Context, id int) error {
					return tc.repoDeleteByIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			err := serverSvc.DeleteByHostname(context.Background(), tc.hostnameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
