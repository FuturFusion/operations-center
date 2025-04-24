package provisioning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestServerService_Create(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

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
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			server: provisioning.Server{
				Name:          "", // invalid
				Cluster:       ptr.To("one"),
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
				Cluster:       ptr.To("one"),
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
				CreateFunc: func(ctx context.Context, in provisioning.Server) (int64, error) {
					require.Equal(t, fixedDate, in.LastUpdated)
					return 1, tc.repoCreateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

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
			repo := &mock.ServerRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

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

func TestServerService_GetAllIDsWithFilter(t *testing.T) {
	tests := []struct {
		name                       string
		filter                     provisioning.ServerFilter
		repoGetAllIDsWithFilter    []string
		repoGetAllIDsWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success - no filter expression",
			filter: provisioning.ServerFilter{
				Cluster: ptr.To("one"),
			},
			repoGetAllIDsWithFilter: []string{
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
			repoGetAllIDsWithFilter: []string{
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
			repoGetAllIDsWithFilter: []string{
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
			repoGetAllIDsWithFilter: []string{
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
			repoGetAllIDsWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
			count: 0,
		},
		{
			name:                       "error - repo",
			repoGetAllIDsWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllIDsWithFilter, tc.repoGetAllIDsWithFilterErr
				},
				GetAllNamesWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
					return tc.repoGetAllIDsWithFilter, tc.repoGetAllIDsWithFilterErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			serverIDs, err := serverSvc.GetAllNamesWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverIDs, tc.count)
		})
	}
}

func TestServerService_GetByID(t *testing.T) {
	tests := []struct {
		name              string
		idArg             string
		repoGetByIDServer *provisioning.Server
		repoGetByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: "one",
			repoGetByIDServer: &provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:           "error - repo",
			idArg:          "one",
			repoGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByIDServer, tc.repoGetByIDErr
				},
			}

			serverSvc := provisioning.NewServerService(repo)

			// Run test
			server, err := serverSvc.GetByName(context.Background(), tc.idArg)

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
		repoGetByIDServer *provisioning.Server
		repoGetByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByIDServer: &provisioning.Server{
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
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
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

func TestServerService_Update(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name          string
		nameArg       string
		server        provisioning.Server
		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			server: provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - validation",
			nameArg: "one",
			server: provisioning.Server{
				Name:          "", // invalid
				Cluster:       ptr.To("one"),
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - repo.UpdateByID",
			nameArg: "one",
			server: provisioning.Server{
				Name:          "one",
				Cluster:       ptr.To("one"),
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
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					require.Equal(t, fixedDate, in.LastUpdated)
					return tc.repoUpdateErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			err := serverSvc.Update(context.Background(), tc.server)

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
			name:          "error - repo.GetByName",
			oldName:       "one",
			newName:       "one",
			repoRenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ServerRepoMock{
				RenameFunc: func(ctx context.Context, oldName string, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameErr
				},
			}

			serverSvc := provisioning.NewServerService(repo, provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			err := serverSvc.Rename(context.Background(), tc.oldName, tc.newName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_DeleteByName(t *testing.T) {
	tests := []struct {
		name              string
		nameArg           string
		repoDeleteByIDErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",

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
				DeleteByNameFunc: func(ctx context.Context, name string) error {
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
