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

func TestClusterService_Create(t *testing.T) {
	tests := []struct {
		name          string
		cluster       provisioning.Cluster
		repoCreateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			cluster: provisioning.Cluster{
				Name:            "", // invalid
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			cluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
					return provisioning.Cluster{}, tc.repoCreateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo)

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
					ID:              1,
					Name:            "one",
					ServerHostnames: []string{"server1", "server2"},
					ConnectionURL:   "http://one/",
				},
				provisioning.Cluster{
					ID:              2,
					Name:            "one",
					ServerHostnames: []string{"server1", "server2"},
					ConnectionURL:   "http://one/",
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

			clusterSvc := provisioning.NewClusterService(repo)

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

			clusterSvc := provisioning.NewClusterService(repo)

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
		idArg              int
		repoGetByIDCluster provisioning.Cluster
		repoGetByIDErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: 1,
			repoGetByIDCluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
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
			repo := &mock.ClusterRepoMock{
				GetByIDFunc: func(ctx context.Context, id int) (provisioning.Cluster, error) {
					return tc.repoGetByIDCluster, tc.repoGetByIDErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo)

			// Run test
			cluster, err := clusterSvc.GetByID(context.Background(), tc.idArg)

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
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
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

			clusterSvc := provisioning.NewClusterService(repo)

			// Run test
			cluster, err := clusterSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDCluster, cluster)
		})
	}
}

func TestClusterService_UpdateByName(t *testing.T) {
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
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server3"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoUpdateCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server3"},
				ConnectionURL:   "http://one/",
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
				ID:              1,
				Name:            "one",
				ServerHostnames: nil, // invalid
				ConnectionURL:   "http://one/",
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
				ID:              1,
				Name:            "one 1", // invalid missmatch
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
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
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - cluster shrinking",
			nameArg: "one",
			cluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1"},
				ConnectionURL:   "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrConstraintViolation, a...)
			},
		},
		{
			name:    "error - repo.UpdateByID",
			nameArg: "one",
			cluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server3"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
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
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
					return tc.repoUpdateCluster, tc.repoUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo)

			// Run test
			cluster, err := clusterSvc.UpdateByName(context.Background(), tc.nameArg, tc.cluster)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateCluster, cluster)
		})
	}
}

func TestClusterService_RenameByName(t *testing.T) {
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
				ID:              1,
				Name:            "one new",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1"},
				ConnectionURL:   "http://one/",
			},
			repoUpdateCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one new",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
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
				ID:              1,
				Name:            "", // invalid
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
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
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.UpdateByID",
			nameArg: "one",
			cluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},
			repoGetByNameCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1"},
				ConnectionURL:   "http://one/",
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
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
					require.Equal(t, tc.cluster.Name, in.Name)
					return tc.repoUpdateCluster, tc.repoUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo)

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
		name                 string
		nameArg              string
		repoGetByNameCluster provisioning.Cluster
		repoGetByNameErr     error
		repoDeleteByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameCluster: provisioning.Cluster{
				ID:              1,
				Name:            "one",
				ServerHostnames: []string{"server1"},
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
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
				DeleteByIDFunc: func(ctx context.Context, id int) error {
					return tc.repoDeleteByIDErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo)

			// Run test
			err := clusterSvc.DeleteByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
