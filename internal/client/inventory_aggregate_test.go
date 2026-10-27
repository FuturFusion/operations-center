package client_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetWithFilterInventoryAggregates(t *testing.T) {
	d := daemonSetup(t)

	// seedInventory adds one resource of each of the kinds project, network,
	// storage pool and storage volume to the cluster "clusterOne". Only the
	// storage volume has a parent ("poolOne"), for all the other resources
	// parent_name is NULL.
	seedInventory := func(t *testing.T) {
		t.Helper()

		_, err := provisioningEntities.CreateCluster(t.Context(), d.db, provisioning.Cluster{
			Name:    "clusterOne",
			Channel: "stable",
		})
		require.NoError(t, err)

		_, err = entities.CreateProject(t.Context(), d.db, inventory.Project{
			UUID:    uuidgen.FromPattern(t, "1"),
			Name:    "default",
			Cluster: "clusterOne",
		})
		require.NoError(t, err)

		_, err = entities.CreateNetwork(t.Context(), d.db, inventory.Network{
			UUID:        uuidgen.FromPattern(t, "2"),
			Name:        "netOne",
			Cluster:     "clusterOne",
			ProjectName: "default",
		})
		require.NoError(t, err)

		_, err = entities.CreateStoragePool(t.Context(), d.db, inventory.StoragePool{
			UUID:    uuidgen.FromPattern(t, "3"),
			Name:    "poolOne",
			Cluster: "clusterOne",
		})
		require.NoError(t, err)

		_, err = entities.CreateStorageVolume(t.Context(), d.db, inventory.StorageVolume{
			UUID:            uuidgen.FromPattern(t, "4"),
			Name:            "volOne",
			Cluster:         "clusterOne",
			ProjectName:     "default",
			StoragePoolName: "poolOne",
			Type:            "custom",
		})
		require.NoError(t, err)
	}

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		filter api.InventoryAggregateFilter

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.InventoryAggregate)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:       "success - all resources of a cluster",
			client:     d.socketClient,
			dbSeedFunc: seedInventory,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "clusterOne", result[0].Cluster)
				require.Len(t, result[0].Projects, 1)
				require.Equal(t, "default", result[0].Projects[0].Name)
				require.Len(t, result[0].Networks, 1)
				require.Equal(t, "netOne", result[0].Networks[0].Name)
				require.Len(t, result[0].StoragePools, 1)
				require.Equal(t, "poolOne", result[0].StoragePools[0].Name)
				require.Len(t, result[0].StorageVolumes, 1)
				require.Empty(t, result[0].Images)
				require.Empty(t, result[0].Instances)
			},
		},
		{
			name:       "success - filter by kind",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: api.InventoryAggregateFilter{
				Kinds: []string{"network"},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Len(t, result[0].Networks, 1)
				require.Empty(t, result[0].Projects)
				require.Empty(t, result[0].StoragePools)
				require.Empty(t, result[0].StorageVolumes)
			},
		},
		{
			name:       "success - filter by cluster without match",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: api.InventoryAggregateFilter{
				Clusters: []string{"unknown"},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:       "success - filter by expression",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: api.InventoryAggregateFilter{
				Expression: new(`name == "netOne"`),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Len(t, result[0].Networks, 1)
				require.Equal(t, "netOne", result[0].Networks[0].Name)
				require.Empty(t, result[0].Projects)
			},
		},
		{
			name:       "success - filter by parent",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: api.InventoryAggregateFilter{
				Parents: []string{"poolOne"},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Len(t, result[0].StorageVolumes, 1)
				require.Empty(t, result[0].Projects)
				require.Empty(t, result[0].Networks)
				require.Empty(t, result[0].StoragePools)
			},
		},
		{
			name:       "success - filter by parent, including resources without parent",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: api.InventoryAggregateFilter{
				Parents:           []string{"poolOne"},
				ParentIncludeNull: true,
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Len(t, result[0].StorageVolumes, 1)
				require.Len(t, result[0].Projects, 1)
				require.Len(t, result[0].Networks, 1)
				require.Len(t, result[0].StoragePools, 1)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.InventoryAggregate) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetWithFilterInventoryAggregates(t.Context(), tc.filter)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetInventoryAggregates_invalidIncludeNull(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name  string
		query url.Values
	}{
		{
			name:  "server_include_null",
			query: url.Values{"server_include_null": {"nope"}},
		},
		{
			name:  "project_include_null",
			query: url.Values{"project_include_null": {"nope"}},
		},
		{
			name:  "parent_include_null",
			query: url.Values{"parent_include_null": {"nope"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := d.socketClient.DoRequest(t.Context(), http.MethodGet, "/inventory/query", tc.query, nil)

			errassert.StatusErrorContains(http.StatusBadRequest, `Invalid value "nope" for query parameter "`+tc.name+`"`)(t, err, "the typed client can not send an invalid bool, so use a raw request")
		})
	}
}
