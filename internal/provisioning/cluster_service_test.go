package provisioning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	serviceMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestClusterService_Create(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                     string
		cluster                  provisioning.Cluster
		serverSvcGetByName       provisioning.Server
		serverSvcGetByNameErr    error
		serverSvcUpdateByName    provisioning.Server
		serverSvcUpdateByNameErr error
		repoCreateErr            error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetByName: provisioning.Server{
				Name: "server1",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			cluster: provisioning.Cluster{
				Name:          "", // invalid
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - serverSvc.GetByName",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - server already part of cluster",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetByName: provisioning.Server{
				Cluster: "cluster-foo",
				Name:    "server1",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server "server1" is already part of cluster "cluster-foo"`)
			},
		},
		{
			name: "error - repo.Create",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.UpdateByName",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			serverSvcGetByName: provisioning.Server{
				Name: "one",
			},
			serverSvcUpdateByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
					require.Equal(t, fixedDate, in.LastUpdated)
					return provisioning.Cluster{}, tc.repoCreateErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Server, error) {
					return tc.serverSvcGetByName, tc.serverSvcGetByNameErr
				},
				UpdateByNameFunc: func(ctx context.Context, name string, server provisioning.Server) (provisioning.Server, error) {
					return tc.serverSvcUpdateByName, tc.serverSvcUpdateByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, serverSvc, nil, provisioning.ClusterServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			_, err := clusterSvc.Create(context.Background(), tc.cluster)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_GetAll(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllClusters provisioning.Clusters
		repoGetAllErr      error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllClusters: provisioning.Clusters{
				provisioning.Cluster{
					Name:          "one",
					ServerNames:   []string{"server1", "server2"},
					ConnectionURL: "http://one/",
				},
				provisioning.Cluster{
					Name:          "one",
					ServerNames:   []string{"server1", "server2"},
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
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllClusters, tc.repoGetAllErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil)

			// Run test
			clusters, err := clusterSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusters, tc.count)
		})
	}
}

func TestClusterService_GetAllNames(t *testing.T) {
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
			repo := &mock.ClusterRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllIDs, tc.repoGetAllIDsErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil)

			// Run test
			clusterNames, err := clusterSvc.GetAllNames(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterNames, tc.count)
		})
	}
}

func TestClusterService_GetByID(t *testing.T) {
	tests := []struct {
		name               string
		idArg              string
		repoGetByIDCluster provisioning.Cluster
		repoGetByIDErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: "one",
			repoGetByIDCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
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
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Cluster, error) {
					return tc.repoGetByIDCluster, tc.repoGetByIDErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetByName(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDCluster, cluster)
		})
	}
}

func TestClusterService_GetByName(t *testing.T) {
	tests := []struct {
		name               string
		nameArg            string
		repoGetByIDCluster provisioning.Cluster
		repoGetByIDErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByIDCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
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
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Cluster, error) {
					return tc.repoGetByIDCluster, tc.repoGetByIDErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDCluster, cluster)
		})
	}
}

func TestClusterService_UpdateByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                 string
		nameArg              string
		cluster              provisioning.Cluster
		repoGetByNameCluster provisioning.Cluster
		repoGetByNameErr     error
		repoUpdateCluster    provisioning.Cluster
		repoUpdateErr        error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
				ConnectionURL: "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			repoUpdateCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
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
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   nil, // invalid
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
			cluster: provisioning.Cluster{
				Name:          "one 1", // invalid missmatch
				ServerNames:   []string{"server1", "server2"},
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
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
				ConnectionURL: "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				UpdateByNameFunc: func(ctx context.Context, name string, in provisioning.Cluster) (provisioning.Cluster, error) {
					require.Equal(t, fixedDate, in.LastUpdated)
					return tc.repoUpdateCluster, tc.repoUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, provisioning.ClusterServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			cluster, err := clusterSvc.UpdateByName(context.Background(), tc.nameArg, tc.cluster)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateCluster, cluster)
		})
	}
}

func TestClusterService_RenameByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                 string
		nameArg              string
		cluster              provisioning.Cluster
		repoGetByNameCluster provisioning.Cluster
		repoGetByNameErr     error
		repoUpdateCluster    provisioning.Cluster
		repoUpdateErr        error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			cluster: provisioning.Cluster{
				Name:          "one new",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1"},
				ConnectionURL: "http://one/",
			},
			repoUpdateCluster: provisioning.Cluster{
				Name:          "one new",
				ServerNames:   []string{"server1", "server2"},
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
			cluster: provisioning.Cluster{
				Name:          "", // invalid
				ServerNames:   []string{"server1", "server2"},
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
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.UpdateByID",
			nameArg: "one",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1"},
				ConnectionURL: "http://one/",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
				UpdateByNameFunc: func(ctx context.Context, name string, in provisioning.Cluster) (provisioning.Cluster, error) {
					require.Equal(t, tc.cluster.Name, in.Name)
					require.Equal(t, fixedDate, in.LastUpdated)
					return tc.repoUpdateCluster, tc.repoUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, provisioning.ClusterServiceWithNow(func() time.Time { return fixedDate }))

			// Run test
			cluster, err := clusterSvc.RenameByName(context.Background(), tc.nameArg, tc.cluster)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateCluster, cluster)
		})
	}
}

func TestClusterService_DeleteByName(t *testing.T) {
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
			repo := &mock.ClusterRepoMock{
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByIDErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil)

			// Run test
			err := clusterSvc.DeleteByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_ResyncInventoryByName(t *testing.T) {
	tests := []struct {
		name                 string
		nameArg              string
		repoGetByNameCluster provisioning.Cluster
		repoGetByNameErr     error
		inventorySyncerErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameCluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1"},
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
			name:    "error - sync cluster",
			nameArg: "one",
			repoGetByNameCluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1"},
			},
			inventorySyncerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
			}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				SyncClusterFunc: func(ctx context.Context, clusterName string) error {
					return tc.inventorySyncerErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil)
			clusterSvc.SetInventorySyncers([]provisioning.InventorySyncer{inventorySyncer})

			// Run test
			err := clusterSvc.ResyncInventoryByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
