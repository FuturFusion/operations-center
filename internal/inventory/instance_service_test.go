package inventory_test

import (
	"context"
	"testing"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/inventory"
	serviceMock "github.com/FuturFusion/operations-center/internal/inventory/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/inventory/repo/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestInstanceService_GetAllIDs(t *testing.T) {
	tests := []struct {
		name             string
		repoGetAllIDs    []int
		repoGetAllIDsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllIDs: []int{
				1, 2,
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
			repo := &repoMock.InstanceRepoMock{
				GetAllIDsFunc: func(ctx context.Context) ([]int, error) {
					return tc.repoGetAllIDs, tc.repoGetAllIDsErr
				},
			}

			instanceSvc := inventory.NewInstanceService(repo, nil, nil, nil)

			// Run test
			instanceHostnames, err := instanceSvc.GetAllIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, instanceHostnames, tc.count)
		})
	}
}

func TestInstanceService_GetByID(t *testing.T) {
	tests := []struct {
		name                string
		idArg               int
		repoGetByIDInstance inventory.Instance
		repoGetByIDErr      error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: 1,
			repoGetByIDInstance: inventory.Instance{
				ID:          1,
				ServerID:    1,
				ProjectName: "one",
				Name:        "one",
				Object:      incusapi.InstanceFull{},
				LastUpdated: time.Now(),
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
			repo := &repoMock.InstanceRepoMock{
				GetByIDFunc: func(ctx context.Context, id int) (inventory.Instance, error) {
					return tc.repoGetByIDInstance, tc.repoGetByIDErr
				},
			}

			instanceSvc := inventory.NewInstanceService(repo, nil, nil, nil)

			// Run test
			instance, err := instanceSvc.GetByID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDInstance, instance)
		})
	}
}

func TestInstanceService_SyncAll(t *testing.T) {
	// Includes also SyncCluster and SyncServer
	tests := []struct {
		name                              string
		clusterSvcGetAllClusters          provisioning.Clusters
		clusterSvcGetAllErr               error
		serverSvcGetAllByClusterIDServers provisioning.Servers
		serverSvcGetAllByClusterIDErr     error
		serverSvcGetByIDServer            provisioning.Server
		serverSvcGetByIDErr               error
		instanceClientProviderErr         error
		instanceClientGetInstances        []incusapi.InstanceFull
		instanceClientGetInstancesErr     error
		repoDeleteByServerIDErr           error
		repoCreateErr                     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDServers: provisioning.Servers{
				{
					ID:        1,
					ClusterID: 1,
					Hostname:  "server-one",
				},
			},
			serverSvcGetByIDServer: provisioning.Server{
				ID:        1,
				ClusterID: 1,
				Hostname:  "server-one",
			},
			instanceClientGetInstances: []incusapi.InstanceFull{
				{
					Instance: incusapi.Instance{
						Name: "instance one",
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                "error - cluster service get all",
			clusterSvcGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - server service get all by cluster ID",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - server service get by ID",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDServers: provisioning.Servers{
				{
					ID:        1,
					ClusterID: 1,
					Hostname:  "server-one",
				},
			},
			serverSvcGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - instance client provider",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDServers: provisioning.Servers{
				{
					ID:        1,
					ClusterID: 1,
					Hostname:  "server-one",
				},
			},
			serverSvcGetByIDServer: provisioning.Server{
				ID:        1,
				ClusterID: 1,
				Hostname:  "server-one",
			},
			instanceClientProviderErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - instance client get instances",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDServers: provisioning.Servers{
				{
					ID:        1,
					ClusterID: 1,
					Hostname:  "server-one",
				},
			},
			serverSvcGetByIDServer: provisioning.Server{
				ID:        1,
				ClusterID: 1,
				Hostname:  "server-one",
			},
			instanceClientGetInstancesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - instances delete by server ID",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDServers: provisioning.Servers{
				{
					ID:        1,
					ClusterID: 1,
					Hostname:  "server-one",
				},
			},
			serverSvcGetByIDServer: provisioning.Server{
				ID:        1,
				ClusterID: 1,
				Hostname:  "server-one",
			},
			instanceClientGetInstances: []incusapi.InstanceFull{
				{
					Instance: incusapi.Instance{
						Name: "instance one",
					},
				},
			},
			repoDeleteByServerIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - instance create",
			clusterSvcGetAllClusters: provisioning.Clusters{
				{
					ID:   1,
					Name: "cluster one",
				},
			},
			serverSvcGetAllByClusterIDServers: provisioning.Servers{
				{
					ID:        1,
					ClusterID: 1,
					Hostname:  "server-one",
				},
			},
			serverSvcGetByIDServer: provisioning.Server{
				ID:        1,
				ClusterID: 1,
				Hostname:  "server-one",
			},
			instanceClientGetInstances: []incusapi.InstanceFull{
				{
					Instance: incusapi.Instance{
						Name: "instance one",
					},
				},
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.InstanceRepoMock{
				DeleteByServerIDFunc: func(ctx context.Context, serverID int) error {
					return tc.repoDeleteByServerIDErr
				},
				CreateFunc: func(ctx context.Context, instance inventory.Instance) (inventory.Instance, error) {
					return inventory.Instance{}, tc.repoCreateErr
				},
			}

			clusterSvc := &serviceMock.ClusterServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.clusterSvcGetAllClusters, tc.clusterSvcGetAllErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllByClusterIDFunc: func(ctx context.Context, clusterID int) (provisioning.Servers, error) {
					return tc.serverSvcGetAllByClusterIDServers, tc.serverSvcGetAllByClusterIDErr
				},
				GetByIDFunc: func(ctx context.Context, id int) (provisioning.Server, error) {
					return tc.serverSvcGetByIDServer, tc.serverSvcGetByIDErr
				},
			}

			instanceClientProvider := func(ctx context.Context, connectionURL string) (inventory.ServerClient, error) {
				return &serviceMock.ServerClientMock{
					GetInstancesFunc: func(ctx context.Context) ([]incusapi.InstanceFull, error) {
						return tc.instanceClientGetInstances, tc.instanceClientGetInstancesErr
					},
				}, tc.instanceClientProviderErr
			}

			instanceSvc := inventory.NewInstanceService(repo, clusterSvc, serverSvc, instanceClientProvider)

			// Run test
			err := instanceSvc.SyncAll(context.Background())

			// Assert
			tc.assertErr(t, err)
		})
	}
}
