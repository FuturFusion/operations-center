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
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				ClusterID:     1,
				ConnectionURL: "http://one/",
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
				ClusterID:     1,
				ConnectionURL: "http://one/",
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
					ID:            1,
					Name:          "one",
					ClusterID:     1,
					ConnectionURL: "http://one/",
				},
				provisioning.Server{
					ID:            2,
					Name:          "one",
					ClusterID:     1,
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

func TestServerService_GetAllByClusterID(t *testing.T) {
	tests := []struct {
		name                         string
		clusterIDArg                 int
		repoGetAllByClusterIDServers provisioning.Servers
		repoGetAllByClusterIDErr     error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:         "success",
			clusterIDArg: 1,
			repoGetAllByClusterIDServers: provisioning.Servers{
				provisioning.Server{
					ID:            1,
					Name:          "one",
					ClusterID:     1,
					ConnectionURL: "http://one/",
				},
				provisioning.Server{
					ID:            2,
					Name:          "one",
					ClusterID:     1,
					ConnectionURL: "http://one/",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                     "error - repo",
			clusterIDArg:             1,
			repoGetAllByClusterIDErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetAllByClusterIDFunc: func(ctx context.Context, custerID int) (provisioning.Servers, error) {
					return tc.repoGetAllByClusterIDServers, tc.repoGetAllByClusterIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			servers, err := serverSvc.GetAllByClusterID(context.Background(), tc.clusterIDArg)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, servers, tc.count)
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
			repo := &mock.ServerRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			serverNames, err := serverSvc.GetAllNames(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverNames, tc.count)
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
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
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

func TestServerService_GetByName(t *testing.T) {
	tests := []struct {
		name              string
		nameArg           string
		repoGetByIDServer provisioning.Server
		repoGetByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByIDServer: provisioning.Server{
				Name:          "one",
				ClusterID:     1,
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
			name:           "error - repo",
			nameArg:        "one",
			repoGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Server, error) {
					return tc.repoGetByIDServer, tc.repoGetByIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDServer, server)
		})
	}
}

func TestServerService_UpdateByName(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		server              provisioning.Server
		repoGetByNameServer provisioning.Server
		repoGetByNameErr    error
		repoUpdateServer    provisioning.Server
		repoUpdateErr       error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoGetByNameServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoUpdateServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - validation",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "", // invalid
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - name mismatch",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one 1", // mismatch
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - repo.GetByName",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.UpdateByID",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoGetByNameServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
					return tc.repoUpdateServer, tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.UpdateByName(context.Background(), tc.nameArg, tc.server)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateServer, server)
		})
	}
}

func TestServerService_RenameByName(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		server              provisioning.Server
		repoGetByNameServer provisioning.Server
		repoGetByNameErr    error
		repoUpdateServer    provisioning.Server
		repoUpdateErr       error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoGetByNameServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoUpdateServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - new name empty",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "", // invalid
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - repo.GetByName",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.UpdateByID",
			nameArg: "one",
			server: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoGetByNameServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
				ConnectionURL: "http://one/",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
					require.Equal(t, tc.server.Name, in.Name)
					return tc.repoUpdateServer, tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.RenameByName(context.Background(), tc.nameArg, tc.server)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateServer, server)
		})
	}
}

func TestServerService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		repoGetByNameServer provisioning.Server
		repoGetByNameErr    error
		repoDeleteByIDErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: provisioning.Server{
				ID:            1,
				Name:          "one",
				ClusterID:     1,
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
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:              "error - repo.DeleteByID",
			nameArg:           "one",
			repoDeleteByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				DeleteByIDFunc: func(ctx context.Context, id int) error {
					return tc.repoDeleteByIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			err := serverSvc.DeleteByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
