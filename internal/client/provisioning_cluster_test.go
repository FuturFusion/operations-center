package client_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	localartifactEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/util/testing/certs"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetClusters(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Cluster)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:   "success - one record",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterOne", api.ClusterStatusPending)
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "clusterOne", result[0].Name)
				require.Equal(t, defaultChannelName, result[0].Channel)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetClusters(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetWithFilterClusters(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		filter provisioning.ClusterFilter

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Cluster)
	}{
		{
			name:   "success - filter by expression, matching",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterOne", api.ClusterStatusPending)
				seedCluster(t, d, "clusterTwo", api.ClusterStatusPending)
			},

			filter: provisioning.ClusterFilter{
				Expression: new(`name == "clusterOne"`),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "clusterOne", result[0].Name)
			},
		},
		{
			name:       "success - filter by expression, not matching",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.ClusterFilter{
				Expression: new(`name == "unknown"`),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:       "error - invalid expression",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.ClusterFilter{
				Expression: new("this is not a valid expression"),
			},

			assertErr: require.Error,
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.Cluster) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetWithFilterClusters(t.Context(), tc.filter)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetCluster(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.Cluster)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterOne", api.ClusterStatusPending)
			},

			tcNameArg: "clusterOne",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Cluster) {
				t.Helper()

				require.Equal(t, "clusterOne", result.Name)
				require.Equal(t, "Test cluster clusterOne", result.Description)
				require.Equal(t, api.ClusterStatusPending, result.Status)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterOne",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.Cluster) {
				t.Helper()
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.Cluster) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetCluster(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

// Test_CreateCluster only covers the error paths, creating a cluster bootstraps
// the cluster on the servers through the Incus API.
func Test_CreateCluster(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		cluster api.ClusterPost

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "unauthorized",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
		},
		{
			name:       "error - validation, empty name",
			client:     d.socketClient,
			dbSeedFunc: noop,

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "name can not be empty")
			},
		},
		{
			name:       "error - validation, prohibited characters in name",
			client:     d.socketClient,
			dbSeedFunc: noop,

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "invalid/name",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "name can not contain any of")
			},
		},
		{
			name:       "error - validation, empty list of server names",
			client:     d.socketClient,
			dbSeedFunc: noop,

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "withoutServers",
					ClusterPut: api.ClusterPut{
						Channel: defaultChannelName,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "list of server names can not be empty")
			},
		},
		{
			name:       "error - validation, unknown server type",
			client:     d.socketClient,
			dbSeedFunc: noop,

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "withoutServerType",
					ClusterPut: api.ClusterPut{
						Channel: defaultChannelName,
					},
				},
				ServerNames: []string{"serverOne"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server type can not be")
			},
		},
		{
			name:   "error - conflict",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "existing", api.ClusterStatusPending)
			},

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "existing",
					ClusterPut: api.ClusterPut{
						Channel: defaultChannelName,
					},
				},
				ServerNames: []string{"serverOne"},
				ServerType:  api.ServerTypeIncus,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "already exists")
			},
		},
		{
			name:       "error - unknown server",
			client:     d.socketClient,
			dbSeedFunc: noop,

			cluster: api.ClusterPost{
				Cluster: api.Cluster{
					Name: "withUnknownServer",
					ClusterPut: api.ClusterPut{
						Channel: defaultChannelName,
					},
				},
				ServerNames: []string{"unknown"},
				ServerType:  api.ServerTypeIncus,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CreateCluster(t.Context(), tc.cluster)

			tc.assertErr(t, err)
		})
	}
}

func Test_UpdateCluster(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		cluster   api.ClusterPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			// The cluster does not have any member servers, otherwise the update
			// is propagated to the servers through the Incus API.
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterOne", api.ClusterStatusPending)
			},

			tcNameArg: "clusterOne",
			cluster: api.ClusterPut{
				Channel:       defaultChannelName,
				ConnectionURL: "https://cluster.example.com:6443",
				Description:   "updated description",
				Properties: api.ConfigMap{
					"env": "lab",
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				cluster, err := d.socketClient.GetCluster(t.Context(), "clusterOne")
				require.NoError(t, err)
				require.Equal(t, "updated description", cluster.Description)
				require.Equal(t, "https://cluster.example.com:6443", cluster.ConnectionURL)
				require.Equal(t, api.ConfigMap{"env": "lab"}, cluster.Properties)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterOne",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",
			cluster: api.ClusterPut{
				Channel: defaultChannelName,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, empty channel",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterOne",
			cluster:   api.ClusterPut{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "channel can not be empty")
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, invalid rolling restart restore mode",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterOne",
			cluster: api.ClusterPut{
				Channel: defaultChannelName,
				Config: api.ClusterConfig{
					RollingRestart: api.ClusterConfigRollingRestart{
						RestoreMode: "invalid",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "rolling restart restore mode is invalid")
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateCluster(t.Context(), tc.tcNameArg, tc.cluster)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_RenameCluster(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		tcNewName string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterOne", api.ClusterStatusPending)
			},

			tcNameArg: "clusterOne",
			tcNewName: "clusterRenamed",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				cluster, err := d.socketClient.GetCluster(t.Context(), "clusterRenamed")
				require.NoError(t, err)
				require.Equal(t, "clusterRenamed", cluster.Name)

				_, err = d.socketClient.GetCluster(t.Context(), "clusterOne")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterRenamed",
			tcNewName: "other",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",
			tcNewName: "other",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - new name is empty",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterRenamed",
			tcNewName: "",

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.RenameCluster(t.Context(), tc.tcNameArg, tc.tcNewName)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteCluster(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		tcForce   bool

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success - pending cluster",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "pendingCluster", api.ClusterStatusPending)
			},

			tcNameArg: "pendingCluster",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetCluster(t.Context(), "pendingCluster")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:   "error - cluster in state ready",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "readyCluster", api.ClusterStatusReady)
			},

			tcNameArg: "readyCluster",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Delete for cluster in state "ready"`)
			},
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetCluster(t.Context(), "readyCluster")
				require.NoError(t, err)
			},
		},
		{
			// A forceful delete removes the cluster record regardless of its
			// state, without contacting the cluster.
			name:       "success - forceful delete of cluster in state ready",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "readyCluster",
			tcForce:   true,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetCluster(t.Context(), "readyCluster")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:   "error - cluster with linked servers",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterWithServers", api.ClusterStatusPending)
				seedServer(t, d, "memberServer", new("clusterWithServers"))
			},

			tcNameArg: "clusterWithServers",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "linked servers")
			},
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetCluster(t.Context(), "clusterWithServers")
				require.NoError(t, err)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterWithServers",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteCluster(t.Context(), tc.tcNameArg, tc.tcForce)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_GetClusterArtifacts(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.ClusterArtifact)
	}{
		{
			name:   "success - empty list",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedCluster(t, d, "clusterOne", api.ClusterStatusPending)
			},

			tcNameArg: "clusterOne",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.ClusterArtifact) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			// The cluster has already been seeded by the previous test case.
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedArtifact(t, d, "clusterOne", "artifactOne")
			},

			tcNameArg: "clusterOne",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.ClusterArtifact) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "artifactOne", result[0].Name)
				require.Len(t, result[0].Files, 1)
				require.Equal(t, "artifact.txt", result[0].Files[0].Name)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "clusterOne",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.ClusterArtifact) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetClusterArtifacts(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetClusterArtifact(t *testing.T) {
	d := daemonSetup(t)

	seedClusterArtifact(t, d, "clusterOne", "artifactOne")

	artifact, err := d.socketClient.GetClusterArtifact(t.Context(), "clusterOne", "artifactOne")
	require.NoError(t, err)
	require.Equal(t, "artifactOne", artifact.Name)
	require.Equal(t, "Test artifact artifactOne", artifact.Description)
	require.Len(t, artifact.Files, 1)

	_, err = d.unauthorizedHTTPClient.GetClusterArtifact(t.Context(), "clusterOne", "artifactOne")
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	_, err = d.socketClient.GetClusterArtifact(t.Context(), "clusterOne", "unknown")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_GetClusterArtifactFile(t *testing.T) {
	d := daemonSetup(t)

	seedClusterArtifact(t, d, "clusterOne", "artifactOne")

	rc, err := d.socketClient.GetClusterArtifactFile(t.Context(), "clusterOne", "artifactOne", "artifact.txt")
	require.NoError(t, err)

	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, "artifact content", string(content))

	_, err = d.unauthorizedHTTPClient.GetClusterArtifactFile(t.Context(), "clusterOne", "artifactOne", "artifact.txt")
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	_, err = d.socketClient.GetClusterArtifactFile(t.Context(), "clusterOne", "artifactOne", "unknown.txt")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_GetClusterArtifactArchive(t *testing.T) {
	d := daemonSetup(t)

	seedClusterArtifact(t, d, "clusterOne", "artifactOne")

	rc, err := d.socketClient.GetClusterArtifactArchive(t.Context(), "clusterOne", "artifactOne", "zip")
	require.NoError(t, err)

	defer rc.Close()

	archive, err := io.ReadAll(rc)
	require.NoError(t, err)

	zipReader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	require.Len(t, zipReader.File, 1)
	require.Equal(t, "artifact.txt", zipReader.File[0].Name)

	_, err = d.unauthorizedHTTPClient.GetClusterArtifactArchive(t.Context(), "clusterOne", "artifactOne", "zip")
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	_, err = d.socketClient.GetClusterArtifactArchive(t.Context(), "clusterOne", "artifactOne", "tar")
	require.Error(t, err)
}

// clusterActionRoutes are the routes below /1.0/provisioning/clusters/{name},
// which require the Incus API of the cluster to be reachable. Only the error
// paths, which are handled before the cluster is contacted, are covered.
func clusterActionRoutes(t *testing.T) []struct {
	name string
	call func(ctx context.Context, c client.OperationsCenterClient, clusterName string) error

	// assertUnknownCluster asserts the error returned for a cluster, which does
	// not exist. Not all of the routes look up the cluster first, some reject
	// the request based on the request body or the cluster members before.
	assertUnknownCluster require.ErrorAssertionFunc
} {
	t.Helper()

	_, _, leafPEM, leafKeyPEM := certs.GenerateChain(t)

	assertNotFound := func(tt require.TestingT, err error, a ...any) {
		require.ErrorIs(tt, err, domain.ErrNotFound)
	}

	return []struct {
		name string
		call func(ctx context.Context, c client.OperationsCenterClient, clusterName string) error

		assertUnknownCluster require.ErrorAssertionFunc
	}{
		{"AddServersToCluster", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.AddServersToCluster(ctx, n, []string{"serverOne"}, false, false)
		}, assertNotFound},
		{"RemoveServerFromCluster", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.RemoveServerFromCluster(ctx, n, []string{"serverOne"}, false)
		}, func(tt require.TestingT, err error, a ...any) {
			// The cluster size is checked before the cluster is looked up.
			require.ErrorContains(tt, err, "does not have enough servers for server removal")
		}},
		{"BulkUpdateCluster", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			arguments := json.RawMessage(`{}`)

			return c.BulkUpdateCluster(ctx, n, api.ClusterBulkUpdatePost{
				Action:    api.ClusterBulkUpdateActionUpdateSystemKernel,
				Arguments: &arguments,
			})
		}, assertNotFound},
		{"ResyncCluster", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.ResyncCluster(ctx, n)
		}, func(tt require.TestingT, err error, a ...any) {
			// The inventory syncers are run per resource kind and each of them
			// reports the missing cluster members individually.
			require.ErrorContains(tt, err, "cluster does not have any servers")
		}},
		{"LaunchClusterWideUpdate", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.LaunchClusterWideUpdate(ctx, n, api.ClusterUpdatePost{})
		}, assertNotFound},
		{"LaunchClusterWideReboot", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.LaunchClusterWideReboot(ctx, n)
		}, assertNotFound},
		{"CancelClusterWideOperation", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.CancelClusterWideOperation(ctx, n)
		}, assertNotFound},
		{"UpdateClusterCertificate", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.UpdateClusterCertificate(ctx, n, api.ClusterCertificatePut{
				ClusterCertificate:    string(leafPEM),
				ClusterCertificateKey: string(leafKeyPEM),
			})
		}, func(tt require.TestingT, err error, a ...any) {
			// The cluster members are resolved before the cluster is looked up.
			require.ErrorContains(tt, err, "cluster does not have any servers")
		}},
	}
}

// Test_BulkUpdateCluster_withoutArguments asserts, that a bulk update request
// without arguments is rejected. All of the supported actions require
// arguments and used to dereference them unconditionally.
func Test_BulkUpdateCluster_withoutArguments(t *testing.T) {
	d := daemonSetup(t)

	seedCluster(t, d, "clusterOne", api.ClusterStatusPending)

	err := d.socketClient.BulkUpdateCluster(t.Context(), "clusterOne", api.ClusterBulkUpdatePost{
		Action: api.ClusterBulkUpdateActionUpdateSystemKernel,
	})
	require.ErrorContains(t, err, `Missing arguments for action "update_system_kernel"`)

	// The daemon is still responsive.
	_, err = d.socketClient.GetCluster(t.Context(), "clusterOne")
	require.NoError(t, err)
}

func Test_ClusterActionRoutes_notAuthorized(t *testing.T) {
	d := daemonSetup(t)

	for _, tc := range clusterActionRoutes(t) {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t.Context(), d.unauthorizedHTTPClient, "clusterOne")

			require.ErrorIs(t, err, domain.ErrNotAuthenticated)
		})
	}
}

func Test_ClusterActionRoutes_unknownCluster(t *testing.T) {
	d := daemonSetup(t)

	for _, tc := range clusterActionRoutes(t) {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t.Context(), d.socketClient, "unknown")

			tc.assertUnknownCluster(t, err)
		})
	}
}

func seedCluster(t *testing.T, d testDaemon, name string, status api.ClusterStatus) {
	t.Helper()

	_, err := entities.CreateCluster(t.Context(), d.db, provisioning.Cluster{
		Name:        name,
		Channel:     defaultChannelName,
		Status:      status,
		Description: "Test cluster " + name,
	})
	require.NoError(t, err)
}

// seedClusterArtifact adds a cluster together with an artifact holding a single
// file.
func seedClusterArtifact(t *testing.T, d testDaemon, clusterName string, artifactName string) {
	t.Helper()

	seedCluster(t, d, clusterName, api.ClusterStatusPending)
	seedArtifact(t, d, clusterName, artifactName)
}

// seedArtifact adds an artifact holding a single file to an existing cluster.
func seedArtifact(t *testing.T, d testDaemon, clusterName string, artifactName string) {
	t.Helper()

	const content = "artifact content"

	_, err := localartifactEntities.CreateClusterArtifact(t.Context(), d.db, provisioning.ClusterArtifact{
		Cluster:     clusterName,
		Name:        artifactName,
		Description: "Test artifact " + artifactName,
		Files: provisioning.ClusterArtifactFiles{
			{
				Name:     "artifact.txt",
				MimeType: "text/plain",
				Size:     int64(len(content)),
			},
		},
	})
	require.NoError(t, err)

	seedFile(t, d, filepath.Join("artifacts", clusterName, artifactName, "artifact.txt"), []byte(content))
}
