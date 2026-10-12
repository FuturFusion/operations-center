package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/inventory"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema/seed"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
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

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
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

	wantInstanceUUIDs, err := inventorySqlite.NewInstance(db).GetAllUUIDsWithFilter(ctx, inventory.InstanceFilter{
		Cluster: new("cluster-00000001"),
	})
	require.NoError(t, err)

	gotInstanceUUIDs := make([]uuid.UUID, 0, len(aggregate.Instances))
	for _, instance := range aggregate.Instances {
		gotInstanceUUIDs = append(gotInstanceUUIDs, instance.UUID)
	}

	require.ElementsMatch(t, wantInstanceUUIDs, gotInstanceUUIDs, "instance UUIDs match the per kind repository")

	for _, item := range aggregate.Images {
		require.NotEqual(t, uuid.Nil, item.UUID, "image %q has a UUID", item.Name)
	}

	for _, item := range aggregate.Instances {
		require.NotEqual(t, uuid.Nil, item.UUID, "instance %q has a UUID", item.Name)
	}

	for _, item := range aggregate.Networks {
		require.NotEqual(t, uuid.Nil, item.UUID, "network %q has a UUID", item.Name)
	}

	for _, item := range aggregate.NetworkACLs {
		require.NotEqual(t, uuid.Nil, item.UUID, "network ACL %q has a UUID", item.Name)
	}

	for _, item := range aggregate.NetworkForwards {
		require.NotEqual(t, uuid.Nil, item.UUID, "network forward %q has a UUID", item.Name)
	}

	for _, item := range aggregate.NetworkIntegrations {
		require.NotEqual(t, uuid.Nil, item.UUID, "network integration %q has a UUID", item.Name)
	}

	for _, item := range aggregate.NetworkLoadBalancers {
		require.NotEqual(t, uuid.Nil, item.UUID, "network load balancer %q has a UUID", item.Name)
	}

	for _, item := range aggregate.NetworkPeers {
		require.NotEqual(t, uuid.Nil, item.UUID, "network peer %q has a UUID", item.Name)
	}

	for _, item := range aggregate.NetworkZones {
		require.NotEqual(t, uuid.Nil, item.UUID, "network zone %q has a UUID", item.Name)
	}

	for _, item := range aggregate.Profiles {
		require.NotEqual(t, uuid.Nil, item.UUID, "profile %q has a UUID", item.Name)
	}

	for _, item := range aggregate.Projects {
		require.NotEqual(t, uuid.Nil, item.UUID, "project %q has a UUID", item.Name)
	}

	for _, item := range aggregate.StorageBuckets {
		require.NotEqual(t, uuid.Nil, item.UUID, "storage bucket %q has a UUID", item.Name)
	}

	for _, item := range aggregate.StoragePools {
		require.NotEqual(t, uuid.Nil, item.UUID, "storage pool %q has a UUID", item.Name)
	}

	for _, item := range aggregate.StorageVolumes {
		require.NotEqual(t, uuid.Nil, item.UUID, "storage volume %q has a UUID", item.Name)
	}
}
