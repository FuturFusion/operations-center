package provisioning_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
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
		name                           string
		cluster                        provisioning.Cluster
		repoExistsByName               bool
		repoExistsByNameErr            error
		repoCreateErr                  error
		repoUpdateErr                  error
		clientPingErr                  error
		clientEnableOSServiceErr       error
		clientSetServerConfig          []queue.Item[struct{}]
		clientEnableClusterCertificate string
		clientEnableClusterErr         error
		clientGetClusterNodeNamesErr   error
		clientGetClusterJoinToken      string
		clientGetClusterJoinTokenErr   error
		clientJoinClusterErr           error
		clientGetOSData                api.OSData
		clientGetOSDataErr             error
		serverSvcGetByName             []queue.Item[*provisioning.Server]
		serverSvcUpdateErr             error
		provisionerApplyErr            error
		provisionerInitErr             error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerNames: []string{"server1", "server2"},
				ServerType:  api.ServerTypeIncus,
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": true,
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			repoExistsByName: true, // cluster with the same name already exists

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Cluster with name "one" already exists`)
			},
		},
		{
			name: "error - repo.ExistsByName",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			repoExistsByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.GetByName",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Cluster: ptr.To("cluster-foo"), // already part of cluster.
						Name:    "server1",
						Type:    api.ServerTypeIncus,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" is already part of cluster "cluster-foo"`)
			},
		},
		{
			name: "error - repo.Create",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},
			clientEnableClusterCertificate: "certificate",
			repoCreateErr:                  boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - server has wrong type",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeMigrationManager, // wrong type, incus expected.
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" has type "migration-manager" but "incus" was expected`)
			},
		},
		{
			name: "error - client.Ping",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - invalid os service config",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": []string{}, // invalid, not a map[string]any

				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to enable OS service "lvm" on "server1": config is not an object`)
			},
		},
		{
			name: "error - lvm enabled not bool",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": "", // invalid, not bool
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to enable OS service "lvm" on "server1": "enabled" is not a bool`)
			},
		},
		{
			name: "error - invalid os service config",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": true,
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						ID:   2001, // invalid, server ID must not be > 2000 for LVM system_id.
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to enable OS service "lvm" on "server1": can not enable LVM on servers with internal ID > 2000`)
			},
		},
		{
			name: "error - client.EnableOSService",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
				ServicesConfig: map[string]any{
					"lvm": map[string]any{
						"enabled": true,
					},
				},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},
			clientEnableOSServiceErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.SetServerConfig",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Cluster: ptr.To("cluster-foo"), // added to a cluster since the first check.
						Name:    "server1",
						Type:    api.ServerTypeIncus,
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Server "server1" was not part of a cluster, but is now part of "cluster-foo"`)
			},
		},
		{
			name: "error - repo.Update",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
			name: "error - client.GetOSData",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
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
			name: "error - provisioner.Init",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerInitErr:             boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - provisioner.Apply",
			cluster: provisioning.Cluster{
				Name:        "one",
				ServerType:  api.ServerTypeIncus,
				ServerNames: []string{"server1", "server2"},
			},
			serverSvcGetByName: []queue.Item[*provisioning.Server]{
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server1",
						Type: api.ServerTypeIncus,
					},
				},
				{
					Value: &provisioning.Server{
						Name: "server2",
						Type: api.ServerTypeIncus,
					},
				},
			},
			clientSetServerConfig: []queue.Item[struct{}]{
				{}, // Server 1
				{}, // Server 2
			},
			clientEnableClusterCertificate: "certificate",
			provisionerApplyErr:            boom.Error,

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
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				EnableOSServiceFunc: func(ctx context.Context, server provisioning.Server, name string, config map[string]any) error {
					return tc.clientEnableOSServiceErr
				},
				SetServerConfigFunc: func(ctx context.Context, endpoint provisioning.Endpoint, config map[string]string) error {
					_, err := queue.Pop(t, &tc.clientSetServerConfig)
					return err
				},
				EnableClusterFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
					return tc.clientEnableClusterCertificate, tc.clientEnableClusterErr
				},
				GetClusterNodeNamesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) ([]string, error) {
					return []string{"one"}, tc.clientGetClusterNodeNamesErr
				},
				GetClusterJoinTokenFunc: func(ctx context.Context, endpoint provisioning.Endpoint, memberName string) (string, error) {
					return tc.clientGetClusterJoinToken, tc.clientGetClusterJoinTokenErr
				},
				JoinClusterFunc: func(ctx context.Context, server provisioning.Server, joinToken string, endpoint provisioning.Endpoint) error {
					return tc.clientJoinClusterErr
				},
				GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
					return tc.clientGetOSData, tc.clientGetOSDataErr
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

			provisioner := &adapterMock.ClusterProvisioningPortMock{
				RegisterUpdateSignalFunc: func(signal signals.Signal[provisioning.ClusterUpdateMessage]) {},
				InitFunc: func(ctx context.Context, name string, config provisioning.ClusterProvisioningConfig) error {
					return tc.provisionerInitErr
				},
				ApplyFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					return tc.provisionerApplyErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, serverSvc, nil, provisioner,
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

func TestClusterService_GetProvisionerConfigurationArchive(t *testing.T) {
	tests := []struct {
		name                      string
		repoGetByName             *provisioning.Cluster
		repoGetByNameErr          error
		provisionerGetArchiveErr  error
		provisionerGetArchiveRC   io.ReadCloser
		provisionerGetArchiveSize int

		assertErr require.ErrorAssertionFunc
		assert    func(t *testing.T, rc io.ReadCloser, size int)
	}{
		{
			name: "success",
			repoGetByName: &provisioning.Cluster{
				Status: api.ClusterStatusReady,
			},
			provisionerGetArchiveRC:   io.NopCloser(bytes.NewBufferString(`foobar`)),
			provisionerGetArchiveSize: 6,

			assertErr: require.NoError,
			assert: func(t *testing.T, rc io.ReadCloser, size int) {
				t.Helper()

				body, err := io.ReadAll(rc)
				require.NoError(t, err)
				require.Equal(t, []byte(`foobar`), body)
				require.Equal(t, 6, size)
			},
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
			assert: func(t *testing.T, rc io.ReadCloser, size int) {
				t.Helper()

				require.Nil(t, rc)
				require.Zero(t, size)
			},
		},
		{
			name: "error - cluster status not ready",
			repoGetByName: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(t, err, "cluster is not in ready state")
			},
			assert: func(t *testing.T, rc io.ReadCloser, size int) {
				t.Helper()

				require.Nil(t, rc)
				require.Zero(t, size)
			},
		},
		{
			name: "error - provisioner.GetArchive",
			repoGetByName: &provisioning.Cluster{
				Status: api.ClusterStatusReady,
			},
			provisionerGetArchiveErr: boom.Error,

			assertErr: boom.ErrorIs,
			assert: func(t *testing.T, rc io.ReadCloser, size int) {
				t.Helper()

				require.Nil(t, rc)
				require.Zero(t, size)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			provisioner := &adapterMock.ClusterProvisioningPortMock{
				RegisterUpdateSignalFunc: func(signal signals.Signal[provisioning.ClusterUpdateMessage]) {},
				GetArchiveFunc: func(ctx context.Context, name string) (io.ReadCloser, int, error) {
					return tc.provisionerGetArchiveRC, tc.provisionerGetArchiveSize, tc.provisionerGetArchiveErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, provisioner)

			// Run test
			rc, size, err := clusterSvc.GetProvisionerConfigurationArchive(context.Background(), "cluster")

			// Assert
			tc.assertErr(t, err)
			tc.assert(t, rc, size)
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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to compile filter expression:")
			},
			count: 0,
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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to execute filter expression:")
			},
			count: 0,
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
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to compile filter expression:")
			},
			count: 0,
		},
		{
			name: "error - filter expression run",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To(`fromBase64("~invalid")`), // invalid, returns runtime error during evauluation of the expression.
			},
			repoGetAllNamesWithFilter: []string{
				"one",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to execute filter expression:")
			},
			count: 0,
		},
		{
			name: "error - non bool expression",
			filter: provisioning.ClusterFilter{
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
			repo := &mock.ClusterRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNamesWithFilter, tc.repoGetAllNamesWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			cluster: provisioning.Cluster{
				Name:          "one",
				ConnectionURL: ":|\\", // invalid
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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)

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
		deleteMode                          api.ClusterDeleteMode
		repoGetByNameCluster                *provisioning.Cluster
		repoGetByNameErr                    error
		repoDeleteByNameErr                 error
		serverSvcGetAllNamesWithFilterNames []string
		serverSvcGetAllNamesWithFilterErr   error
		serverSvcGetAllWithFilter           provisioning.Servers
		serverSvcGetAllWithFilterErr        error
		clientPingErr                       error
		clientFactoryResetErr               error
		provisionerCleanupErr               error

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
			name:       "success - factory reset",
			nameArg:    "one",
			deleteMode: api.ClusterDeleteModeFactoryReset,
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
				{},
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
			name:                         "error - serverSvc.GetAllWithFilter",
			nameArg:                      "one",
			deleteMode:                   api.ClusterDeleteModeFactoryReset,
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:       "error - client.Ping",
			nameArg:    "one",
			deleteMode: api.ClusterDeleteModeFactoryReset,
			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
			},
			clientPingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:       "error - client.FactoryReset",
			nameArg:    "one",
			deleteMode: api.ClusterDeleteModeFactoryReset,
			serverSvcGetAllWithFilter: provisioning.Servers{
				{},
			},
			clientFactoryResetErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                  "error - provisioner.Cleanup with force or factory-reset",
			nameArg:               "one",
			deleteMode:            api.ClusterDeleteModeForce,
			provisionerCleanupErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                "error - repo.DeleteByID with force or factory-reset",
			nameArg:             "one",
			deleteMode:          api.ClusterDeleteModeForce,
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
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

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, `Delete for cluster in state "ready":`)
			},
		},
		{
			name:                 "error - cluster state not set",
			nameArg:              "one",
			repoGetByNameCluster: &provisioning.Cluster{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, "Delete for cluster with invalid state:")
			},
		},
		{
			name:    "error - cluster with linked servers",
			nameArg: "one",
			repoGetByNameCluster: &provisioning.Cluster{
				Status: api.ClusterStatusPending,
			},
			serverSvcGetAllNamesWithFilterNames: []string{"one"},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
				require.ErrorContains(tt, err, "Delete for cluster with 1 linked servers ([one])")
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
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientPingErr
				},
				FactoryResetFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return tc.clientFactoryResetErr
				},
			}

			provisioner := &adapterMock.ClusterProvisioningPortMock{
				RegisterUpdateSignalFunc: func(signal signals.Signal[provisioning.ClusterUpdateMessage]) {},
				CleanupFunc: func(ctx context.Context, name string) error {
					return tc.provisionerCleanupErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, serverSvc, nil, provisioner)

			// Run test
			err := clusterSvc.DeleteByName(context.Background(), tc.nameArg, tc.deleteMode)

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

			clusterSvc := provisioning.NewClusterService(repo, nil, nil, nil, nil)
			clusterSvc.SetInventorySyncers(map[domain.ResourceType]provisioning.InventorySyncer{"test": inventorySyncer})

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

			clusterSvc := provisioning.NewClusterService(nil, nil, nil, nil, nil)
			clusterSvc.SetInventorySyncers(map[domain.ResourceType]provisioning.InventorySyncer{"test": inventorySyncer})

			// Run test
			err := clusterSvc.ResyncInventoryByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterService_StartLifecycleEventsMonitor(t *testing.T) {
	doneChannel := func() chan struct{} {
		t.Helper()
		return make(chan struct{})
	}

	doneNonBlocking := func() chan struct{} {
		t.Helper()
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	noLogAssert := func(t *testing.T, logBuf *bytes.Buffer) {
		t.Helper()
	}

	logContains := func(want string) func(t *testing.T, logBuf *bytes.Buffer) {
		return func(t *testing.T, logBuf *bytes.Buffer) {
			t.Helper()

			// Give logs a little bit of time to be processed.
			for range 5 {
				if strings.Contains(logBuf.String(), want) {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			require.Contains(t, logBuf.String(), want)
		}
	}

	tests := []struct {
		name                           string
		initDone                       func() chan struct{}
		repoGetAllClusters             provisioning.Clusters
		repoGetAllErr                  error
		serverSvcGetAllWithFilterErr   error
		clientSubscribeLifecycleEvent  []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]
		inventorySyncerResyncByNameErr error

		assertErr           require.ErrorAssertionFunc
		wantProcessedEvents int
		assertLog           func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:     "success - one cluster and one event",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           noLogAssert,
		},
		{
			name:          "error - GetAll",
			initDone:      doneNonBlocking,
			repoGetAllErr: boom.Error,

			assertErr: require.Error,
			assertLog: noLogAssert,
		},
		{
			name:     "error - GetEndpoint",
			initDone: doneNonBlocking,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: require.NoError,
			assertLog: logContains("Failed to start lifecycle monitor"),
		},
		{
			name:     "error - client.SubscribeLifecycleEvents - ctx.Done",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						return nil, nil, boom.Error
					},
				},
				{
					Value: func(cancel func()) (chan domain.LifecycleEvent, chan error, error) {
						cancel()

						return nil, nil, boom.Error
					},
				},
			},

			assertErr: require.NoError,
			assertLog: logContains("Failed to re-establish event stream"),
		},
		{
			name:     "error - client.SubscribeLifecycleEvents - retry",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						return nil, nil, boom.Error
					},
				},
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           logContains("Failed to re-establish event stream"),
		},
		{
			name:     "error - unavailable inventory syncer",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 2)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceType("unavailable"), // unavailable inventory syncer
						}

						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},
			inventorySyncerResyncByNameErr: boom.Error,

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           logContains("No inventory syncer available for the resource type"),
		},
		{
			name:     "error - inventorySyncer.ResyncByName",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},
			inventorySyncerResyncByNameErr: boom.Error,

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           logContains("Failed to resync"),
		},
		{
			name:     "error - Lifecycle subscription ended",
			initDone: doneChannel,
			repoGetAllClusters: provisioning.Clusters{
				{
					Name: "one",
				},
			},
			clientSubscribeLifecycleEvent: []queue.Item[func(cancel func()) (chan domain.LifecycleEvent, chan error, error)]{
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						errChan := make(chan error, 1)
						errChan <- boom.Error

						return nil, errChan, nil
					},
				},
				{
					Value: func(_ func()) (chan domain.LifecycleEvent, chan error, error) {
						t.Helper()

						events := make(chan domain.LifecycleEvent, 1)
						events <- domain.LifecycleEvent{
							ResourceType: domain.ResourceTypeImage,
						}

						return events, nil, nil
					},
				},
			},

			assertErr:           require.NoError,
			wantProcessedEvents: 1,
			assertLog:           logContains("Lifecycle events subscription ended"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cancableCtx, cancel := context.WithCancel(t.Context())
			defer cancel()

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			done := tc.initDone()

			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return tc.repoGetAllClusters, tc.repoGetAllErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return provisioning.Servers{}, tc.serverSvcGetAllWithFilterErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				SubscribeLifecycleEventsFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (chan domain.LifecycleEvent, chan error, error) {
					call, _ := queue.PopRetainLast(t, &tc.clientSubscribeLifecycleEvent)
					return call(cancel)
				},
			}

			processedEvents := 0
			processedEventsMu := sync.Mutex{}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				ResyncByNameFunc: func(ctx context.Context, clusterName string, sourceDetails domain.LifecycleEvent) error {
					processedEventsMu.Lock()
					defer processedEventsMu.Unlock()

					processedEvents++

					if processedEvents == tc.wantProcessedEvents {
						defer close(done)
					}

					return tc.inventorySyncerResyncByNameErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, serverSvc, map[domain.ResourceType]provisioning.InventorySyncer{domain.ResourceTypeImage: inventorySyncer}, nil)

			// Run test
			err = clusterSvc.StartLifecycleEventsMonitor(cancableCtx)

			select {
			case <-done:
				cancel()
			case <-cancableCtx.Done():
			case <-t.Context().Done():
				t.Fatal("Test context cancelled before test ended")
			case <-time.After(1000 * time.Millisecond):
				cancel()
				t.Error("Test timeout reached before test ended")
			}

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantProcessedEvents, processedEvents)
			tc.assertLog(t, logBuf)
		})
	}
}

func TestClusterService_StartLifecycleEventsMonitor_AddListener(t *testing.T) {
	noLogAssert := func(t *testing.T, logBuf string) {
		t.Helper()
	}

	logContains := func(want string) func(t *testing.T, logBuf string) {
		return func(t *testing.T, logBuf string) {
			t.Helper()

			require.Contains(t, logBuf, want)
		}
	}

	tests := []struct {
		name                         string
		serverSvcGetAllWithFilterErr error
		updateMessage                provisioning.ClusterUpdateMessage

		assertLog func(t *testing.T, logBuf string)
	}{
		{
			name: "success register cluster",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationCreate,
				Name:      "new",
			},

			assertLog: noLogAssert,
		},
		{
			name: "error - startLifecycleEventHandler",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationCreate,
				Name:      "new",
			},
			serverSvcGetAllWithFilterErr: boom.Error,

			assertLog: logContains("Failed to start lifecycle monitor"),
		},
		{
			name: "success delete cluster",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationDelete,
				Name:      "existing",
			},

			assertLog: noLogAssert,
		},
		{
			name: "success delete unknown cluster",
			updateMessage: provisioning.ClusterUpdateMessage{
				Operation: provisioning.ClusterUpdateOperationDelete,
				Name:      "unknown",
			},

			assertLog: noLogAssert,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cancableCtx, cancel := context.WithCancel(t.Context())
			defer cancel()

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return provisioning.Clusters{
						{
							Name: "existing",
						},
					}, nil
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return provisioning.Servers{}, tc.serverSvcGetAllWithFilterErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				SubscribeLifecycleEventsFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (chan domain.LifecycleEvent, chan error, error) {
					return nil, nil, nil
				},
			}

			inventorySyncer := &serviceMock.InventorySyncerMock{
				ResyncByNameFunc: func(ctx context.Context, clusterName string, sourceDetails domain.LifecycleEvent) error {
					return nil
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, serverSvc, map[domain.ResourceType]provisioning.InventorySyncer{"test": inventorySyncer}, nil)

			// Run test
			err = clusterSvc.StartLifecycleEventsMonitor(cancableCtx)

			clusterSvc.GetClusterUpdateSignal().Emit(cancableCtx, tc.updateMessage)

			cancel()

			select {
			case <-cancableCtx.Done():
			case <-t.Context().Done():
				t.Fatal("Test context cancelled before test ended")
			case <-time.After(1000 * time.Millisecond):
				cancel()
				t.Error("Test timeout reached before test ended")
			}

			// Assert
			require.NoError(t, err)
			tc.assertLog(t, logBuf.String())
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
		serverSvcGetAllWithFilter         provisioning.Servers
		serverSvcGetAllWithFilterErr      error
		repoGetByName                     *provisioning.Cluster
		repoGetByNameErr                  error
		clientUpdateClusterCertificateErr error
		repoUpdateErr                     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:           "success",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			serverSvcGetAllWithFilter: provisioning.Servers{
				{
					ConnectionURL: "http://one/",
					Certificate:   "cert",
				},
			},
			repoGetByName: &provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name:           "error - invalid certificate pair",
			certificatePEM: "invalid", // invalid
			keyPEM:         "invalid", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(t, err, "Failed to validate key pair:")
			},
		},
		{
			name:                         "error - serverSvc.GetAllWithFilter",
			certificatePEM:               string(certPEM),
			keyPEM:                       string(keyPEM),
			serverSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                              "error - client.UpdateClusterCertificate",
			certificatePEM:                    string(certPEM),
			keyPEM:                            string(keyPEM),
			clientUpdateClusterCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - repo.GetByName",
			certificatePEM:   string(certPEM),
			keyPEM:           string(keyPEM),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - repo.Update",
			certificatePEM: string(certPEM),
			keyPEM:         string(keyPEM),
			repoGetByName: &provisioning.Cluster{
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
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			client := &adapterMock.ClusterClientPortMock{
				UpdateClusterCertificateFunc: func(ctx context.Context, endpoint provisioning.Endpoint, certificatePEM, keyPEM string) error {
					return tc.clientUpdateClusterCertificateErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverSvcGetAllWithFilter, tc.serverSvcGetAllWithFilterErr
				},
			}

			clusterSvc := provisioning.NewClusterService(repo, client, serverSvc, nil, nil)

			// Run test
			err := clusterSvc.UpdateCertificate(context.Background(), "cluster", tc.certificatePEM, tc.keyPEM)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
