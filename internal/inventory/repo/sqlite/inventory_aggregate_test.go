package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/dbschema/seed"
	"github.com/FuturFusion/operations-center/internal/inventory"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func TestInventoryAggregateDatabaseActions(t *testing.T) {
	ctx := context.Background()

	// Create a new temporary database.
	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	err = seed.DB(ctx, db, seed.Config{})
	require.NoError(t, err)

	aggregateRepo := inventorySqlite.NewInventoryAggregate(db)

	aggregates, err := aggregateRepo.GetAllWithFilter(context.Background(), inventory.InventoryAggregateFilter{
		Clusters: []string{"cluster-00000001"},
	})
	require.NoError(t, err)

	require.Len(t, aggregates, 1)

	aggregate := aggregates[0]

	require.Equal(t, "cluster-00000001", aggregate.Cluster)
	require.NotEmpty(t, aggregate.Servers)
	require.NotEmpty(t, aggregate.Images)
	require.NotEmpty(t, aggregate.Instances)
	require.NotEmpty(t, aggregate.Networks)
	require.NotEmpty(t, aggregate.NetworkACLs)
	require.NotEmpty(t, aggregate.NetworkForwards)
	require.NotEmpty(t, aggregate.NetworkIntegrations)
	require.NotEmpty(t, aggregate.NetworkLoadBalancers)
	require.NotEmpty(t, aggregate.NetworkPeers)
	require.NotEmpty(t, aggregate.NetworkZones)
	require.NotEmpty(t, aggregate.Profiles)
	require.NotEmpty(t, aggregate.Projects)
	require.NotEmpty(t, aggregate.StorageBuckets)
	require.NotEmpty(t, aggregate.StoragePools)
	require.NotEmpty(t, aggregate.StorageVolumes)
}
