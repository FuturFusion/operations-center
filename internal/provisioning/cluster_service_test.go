package provisioning_test

import (
	"context"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	serviceMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClusterService_Create(t *testing.T) {
	tests := []struct {
		name                                 string
		cluster                              provisioning.Cluster
		repoExistsByName                     bool
		repoExistsByNameErr                  error
		repoCreateErr                        error
		repoUpdateErr                        error
		clientPingErr                        error
		clientEnableOSServiceLVMErr          error
		clientSetServerConfig                []queue.Item[struct{}]
		clientEnableClusterCertificate       string
		clientEnableClusterErr               error
		clientGetClusterNodeNamesErr         error
		clientGetClusterJoinToken            string
		clientGetClusterJoinTokenErr         error
		clientJoinClusterErr                 error
		clientCreateProjectErr               error
		clientInitializeDefaultStorageErr    error
		clientGetOSData                      api.OSData
		clientGetOSDataErr                   error
		clientInitializeDefaultNetworkingErr error
		serverSvcGetByName                   []queue.Item[*provisioning.Server]
		serverSvcUpdateErr                   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetOSData:                api.OSData{},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			cluster: provisioning.Cluster{
				Name:        "", // invalid
				ServerNames: []string{"server1", "server2"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.ExistsByName cluster already exists",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			repoExistsByName: true, // cluster with the same name already exists

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, `Cluster with name "one" already exists`)
			},
		},
		{
			name: "error - repo.ExistsByName",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			repoExistsByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.GetByName",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - server already part of cluster",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Cluster: ptr.To("cluster-foo"), // already part of cluster.
						Name:    "server1",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server "server1" is already part of cluster "cluster-foo"`)
			},
		},
		{
			name: "error - repo.Create",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientEnableClusterCertificate: "certificate",
			repoCreateErr:                  boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.Ping",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.EnableOSServiceLVM",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientEnableOSServiceLVMErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.SetServerConfig",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				// Server 1
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.EnableCluster",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetClusterNodeNames",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetClusterNodeNamesErr:   boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetClusterJoinToken",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetClusterJoinTokenErr:   boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.JoinCluster",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientJoinClusterErr:           boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.GetByName - 2nd transaction",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Err: boom.Error,
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - server already part of cluster - 2nd transaction",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Cluster: ptr.To("cluster-foo"), // added to a cluster since the first check.
						Name:    "server1",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server "server1" was not part of a cluster, but is now part of "cluster-foo"`)
			},
		},
		{
			name: "error - repo.Update",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			repoUpdateErr:                  boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.Update",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			serverSvcUpdateErr:             boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.CreateProject",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientCreateProjectErr:         boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.InitializeDefaultStorage",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate:    "certificate",
			clientInitializeDefaultStorageErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetOSData",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			clientGetOSDataErr:             boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.InitializeDefaultNetworking",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate:       "certificate",
			clientGetOSData:                      api.OSData{},
			clientInitializeDefaultNetworkingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				ExistsByNameFunc: func(ctx context.Context, name string) (bool, error) {
					return tc.repoExistsByName, tc.repoExistsByNameErr
				},
				CreateFunc: func(ctx context.Context, in provisioning.Cluster) (int64, error) {
					return 0, tc.repoCreateErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				PingFunc: func(ctx context.Context, target provisioning.ServerOrCluster) error {
					return tc.clientPingErr
				},
				EnableOSServiceLVMFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.clientEnableOSServiceLVMErr
				},
				SetServerConfigFunc: func(ctx context.Context, server provisioning.Server, config map[string]string) error {
					_, err := queue.Pop(t, &tc.clientSetServerConfig)
					return err
				},
				EnableClusterFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
					return tc.clientEnableClusterCertificate, tc.clientEnableClusterErr
				},
				GetClusterNodeNamesFunc: func(ctx context.Context, cluster provisioning.Cluster) ([]string, error) {
					return []string{"one"}, tc.clientGetClusterNodeNamesErr
				},
				GetClusterJoinTokenFunc: func(ctx context.Context, cluster provisioning.Cluster, memberName string) (string, error) {
					return tc.clientGetClusterJoinToken, tc.clientGetClusterJoinTokenErr
				},
				JoinClusterFunc: func(ctx context.Context, server provisioning.Server, joinToken string, cluster provisioning.Cluster) error {
					return tc.clientJoinClusterErr
				},
				CreateProjectFunc: func(ctx context.Context, cluster provisioning.Cluster, name string, description string) error {
					return tc.clientCreateProjectErr
				},
				InitializeDefaultStorageFunc: func(ctx context.Context, servers []provisioning.Server) error {
					return tc.clientInitializeDefaultStorageErr
				},
				GetOSDataFunc: func(ctx context.Context, server provisioning.Server) (api.OSData, error) {
					return tc.clientGetOSData, tc.clientGetOSDataErr
				},
				InitializeDefaultNetworkingFunc: func(ctx context.Context, servers []provisioning.Server) error {
					return tc.clientInitializeDefaultNetworkingErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					server, err := queue.Pop(t, &tc.serverSvcGetByName)
					return server, err
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return tc.serverSvcUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, serverSvc, nil,
				provisioning.ClusterServiceCreateClusterRetryTimeout(0),
			)

			// Run test
			_, err := clusterSvc.Create(context.Background(), tc.cluster)

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.clientSetServerConfig)
			require.Empty(t, tc.serverSvcGetByName)
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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			clusters, err := clusterSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusters, tc.count)
		})
	}
}

func TestClusterService_GetAllWithFilter(t *testing.T) {
	tests := []struct {
		name                    string
		filter                  provisioning.ClusterFilter
		repoGetAllWithFilter    provisioning.Clusters
		repoGetAllWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:   "success - no filter expression",
			filter: provisioning.ClusterFilter{},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
				provisioning.Cluster{
					Name: "two",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`Name == "one"`),
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
				provisioning.Cluster{
					Name: "two",
				},
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name: "error - invalid filter expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(``), // the empty expression is an invalid expression.
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
			},

			assertErr: require.Error,
			count:     0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
					Name: "one",
				},
			},

			assertErr: require.Error,
			count:     0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`"string"`), // invalid, does evaluate to string instead of boolean.
			},
			repoGetAllWithFilter: provisioning.Clusters{
				provisioning.Cluster{
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
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetAllWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, cluster, tc.count)
		})
	}
}

func TestClusterService_GetAllNames(t *testing.T) {
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
			repo := &mock.ClusterRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			clusterNames, err := clusterSvc.GetAllNames(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterNames, tc.count)
		})
	}
}

func TestClusterService_GetAllIDsWithFilter(t *testing.T) {
	tests := []struct {
		name                         string
		filter                       provisioning.ClusterFilter
		repoGetAllNamesWithFilter    []string
		repoGetAllNamesWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:   "success - no filter expression",
			filter: provisioning.ClusterFilter{},
			repoGetAllNamesWithFilter: []string{
				"one", "two",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with filter expression",
			filter: provisioning.ClusterFilter{
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
			filter: provisioning.ClusterFilter{
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
			filter: provisioning.ClusterFilter{
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
			filter: provisioning.ClusterFilter{
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
			repo := &mock.ClusterRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNamesWithFilter, tc.repoGetAllNamesWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			clusterIDs, err := clusterSvc.GetAllNamesWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterIDs, tc.count)
		})
	}
}

func TestClusterService_GetByID(t *testing.T) {
	tests := []struct {
		name                 string
		idArg                string
		repoGetByNameCluster *provisioning.Cluster
		repoGetByNameErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo",
			idArg:            "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetByName(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByNameCluster, cluster)
		})
	}
}

func TestClusterService_GetByName(t *testing.T) {
	tests := []struct {
		name                 string
		nameArg              string
		repoGetByNameCluster *provisioning.Cluster
		repoGetByNameErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
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
			name:             "error - repo",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			cluster, err := clusterSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByNameCluster, cluster)
		})
	}
}

func TestClusterService_Update(t *testing.T) {
	tests := []struct {
		name          string
		cluster       provisioning.Cluster
		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
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
			name: "error - repo.Update",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server3"},
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
				UpdateFunc: func(ctx context.Context, in provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			err := clusterSvc.Update(context.Background(), tc.cluster)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_Rename(t *testing.T) {
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
			newName: "one new",

			assertErr: require.NoError,
		},
		{
			name:    "error - old name empty",
			oldName: "", // invalid
			newName: "one new",

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
			name:          "error - repo.Rename",
			oldName:       "one",
			newName:       "one new",
			repoRenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				RenameFunc: func(ctx context.Context, oldName string, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)

			// Run test
			err := clusterSvc.Rename(context.Background(), tc.oldName, tc.newName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                                string
		nameArg                             string
		repoGetByNameCluster                *provisioning.Cluster
		repoGetByNameErr                    error
		repoDeleteByNameErr                 error
		serverSvcGetAllNamesWithFilterNames []string
		serverSvcGetAllNamesWithFilterErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
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
			name:    "error - cluster state ready",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusReady,
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, `Delete for cluster in state "ready" is not allowed`)
			},
		},
		{
			name:                 "error - cluster state not set",
			nameArg:              "one",
			repoGetByNameCluster: &provisioning.Cluster{},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "Delete for cluster with invalid state")
			},
		},
		{
			name:    "error - cluster with linked servers",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			serverSvcGetAllNamesWithFilterNames: []string{"one"},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "Failed to delete cluster: Delete for cluster with 1 linked servers is not allowd ([one])")
			},
		},
		{
			name:    "error - serverSvc.GetallNamesWithFilter",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			serverSvcGetAllNamesWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - repo.DeleteByID",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByNameCluster, tc.repoGetByNameErr
				},
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllNamesWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
					return tc.serverSvcGetAllNamesWithFilterNames, tc.serverSvcGetAllNamesWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, serverSvc, nil)

			// Run test
			err := clusterSvc.DeleteByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_ResyncInventory(t *testing.T) {
	tests := []struct {
		name               string
		ctx                context.Context
		repoGetAllClusters provisioning.Clusters
		repoGetAllErr      error
		inventorySyncerErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success - empty cluster list",
			ctx:                context.Background(),
			repoGetAllClusters: provisioning.Clusters{},

			assertErr: require.NoError,
		},
		{
			name: "success",
			ctx:  context.Background(),
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:          "error - GetAll",
			ctx:           context.Background(),
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - Context done",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // context cancelled
				return ctx
			}(),
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - ResyncInventoryByName",
			ctx:  context.Background(),
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			inventorySyncerErr: boom.Error,

			assertErr: boom.ErrorIs,
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

			inventorySyncer := &serviceMock.InventorySyncerMock{
				SyncClusterFunc: func(ctx context.Context, clusterName string) error {
					return tc.inventorySyncerErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil)
			clusterSvc.SetInventorySyncers([]provisioning.InventorySyncer{inventorySyncer})

			// Run test
			err := clusterSvc.ResyncInventory(tc.ctx)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_ResyncInventoryByName(t *testing.T) {
	tests := []struct {
		name               string
		nameArg            string
		inventorySyncerErr error

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
			name:               "error - sync cluster",
			nameArg:            "one",
			inventorySyncerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inventorySyncer := &serviceMock.InventorySyncerMock{
				SyncClusterFunc: func(ctx context.Context, clusterName string) error {
					return tc.inventorySyncerErr
				},
			}

			clusterSvc := provisioning.NewClusterService(nil, nil, nil, nil)
			clusterSvc.SetInventorySyncers([]provisioning.InventorySyncer{inventorySyncer})

			// Run test
			err := clusterSvc.ResyncInventoryByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_UpdateCertificate(t *testing.T) {
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tests := []struct {
		name                              string
		certificatePEM                    string
		keyPEM                            string
		repoGetByName                     []queue.Item[*provisioning.Cluster]
		clientUpdateClusterCertificateErr error
		repoUpdateErr                     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:           "success",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: []queue.Item[*provisioning.Cluster]{
				{
					Value: &provisioning.Cluster{
						Name:          "one",
						ServerNames:   []string{"server1", "server2"},
						ConnectionURL: "http://one/",
					},
				},
				{
					Value: &provisioning.Cluster{
						Name:          "one",
						ServerNames:   []string{"server1", "server2"},
						ConnectionURL: "http://one/",
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:           "error - invalid certificate pair",
			certificatePEM: "invalid", // invalid
			keyPEM:         "invalid", // invalid

			assertErr: require.Error,
		},
		{
			name:           "error - repo.GetByName",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: []queue.Item[*provisioning.Cluster]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - client.UpdateClusterCertificate",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: []queue.Item[*provisioning.Cluster]{
				{
					Value: &provisioning.Cluster{
						Name:          "one",
						ServerNames:   []string{"server1", "server2"},
						ConnectionURL: "http://one/",
					},
				},
			},
			clientUpdateClusterCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - repo.GetByName 2nd call",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: []queue.Item[*provisioning.Cluster]{
				{
					Value: &provisioning.Cluster{
						Name:          "one",
						ServerNames:   []string{"server1", "server2"},
						ConnectionURL: "http://one/",
					},
				},
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - repo.Update",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: []queue.Item[*provisioning.Cluster]{
				{
					Value: &provisioning.Cluster{
						Name:          "one",
						ServerNames:   []string{"server1", "server2"},
						ConnectionURL: "http://one/",
					},
				},
				{
					Value: &provisioning.Cluster{
						Name:          "one",
						ServerNames:   []string{"server1", "server2"},
						ConnectionURL: "http://one/",
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return queue.Pop(t, &tc.repoGetByName)
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				UpdateClusterCertificateFunc: func(ctx context.Context, cluster provisioning.Cluster, certificatePEM, keyPEM string) error {
					return tc.clientUpdateClusterCertificateErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, nil, nil)

			// Run test
			err := clusterSvc.UpdateCertificate(context.Background(), "cluster", tc.certificatePEM, tc.keyPEM)

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoGetByName)
		})
	}
}
