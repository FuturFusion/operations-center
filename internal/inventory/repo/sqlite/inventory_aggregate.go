package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type inventoryAggregate struct {
	db sqlite.DBTX
}

var _ inventory.InventoryAggregateRepo = &inventoryAggregate{}

func NewInventoryAggregate(db sqlite.DBTX) *inventoryAggregate {
	return &inventoryAggregate{
		db: db,
	}
}

func (r inventoryAggregate) GetAllWithFilter(ctx context.Context, columns inventory.InventoryAggregateColumns, filter inventory.InventoryAggregateFilter) (inventory.InventoryAggregates, error) {
	const sqlStmt = `
SELECT %s FROM inventory
WHERE true
%s
ORDER BY cluster_name
`

	var columnNames []string
	var whereClause []string
	var args []any

	columnNames = append(columnNames, "cluster_name")
	if columns.Servers {
		columnNames = append(columnNames, "servers")
	} else {
		columnNames = append(columnNames, "'[]' AS servers")
	}

	columnNames = append(columnNames, columnPlaceholder(columns.Images, "images"))
	columnNames = append(columnNames, columnPlaceholder(columns.Instances, "instances"))
	columnNames = append(columnNames, columnPlaceholder(columns.Networks, "networks"))
	columnNames = append(columnNames, columnPlaceholder(columns.NetworkACLs, "network_acls"))
	columnNames = append(columnNames, columnPlaceholder(columns.NetworkForwards, "network_forwards"))
	columnNames = append(columnNames, columnPlaceholder(columns.NetworkIntegrations, "network_integrations"))
	columnNames = append(columnNames, columnPlaceholder(columns.NetworkLoadBalancers, "network_load_balancers"))
	columnNames = append(columnNames, columnPlaceholder(columns.NetworkPeers, "network_peers"))
	columnNames = append(columnNames, columnPlaceholder(columns.NetworkZones, "network_zones"))
	columnNames = append(columnNames, columnPlaceholder(columns.Profiles, "profiles"))
	columnNames = append(columnNames, columnPlaceholder(columns.Projects, "projects"))
	columnNames = append(columnNames, columnPlaceholder(columns.StorageBuckets, "storage_buckets"))
	columnNames = append(columnNames, columnPlaceholder(columns.StoragePools, "storage_pools"))
	columnNames = append(columnNames, columnPlaceholder(columns.StorageVolumes, "storage_volumes"))

	if filter.Cluster != nil {
		whereClause = append(whereClause, ` AND cluster_name = :cluster_name`)
		args = append(args, sql.Named("cluster_name", filter.Cluster))
	}

	sqlStmtComplete := fmt.Sprintf(sqlStmt, strings.Join(columnNames, ", "), strings.Join(whereClause, " "))

	rows, err := r.db.QueryContext(ctx, sqlStmtComplete, args...)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var inventoryAggregate inventory.InventoryAggregates
	for rows.Next() {
		var project inventory.InventoryAggregate
		project, err = scanInventoryAggregate(rows)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		inventoryAggregate = append(inventoryAggregate, project)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return inventoryAggregate, nil
}

func columnPlaceholder(realColumn bool, name string) string {
	if realColumn {
		return name
	}

	return "'{}' AS " + name
}

func scanInventoryAggregate(row interface{ Scan(dest ...any) error }) (inventory.InventoryAggregate, error) {
	var (
		servers              []byte
		images               []byte
		instances            []byte
		networks             []byte
		networkACLs          []byte
		networkForwards      []byte
		networkIntegrations  []byte
		networkLoadBalancers []byte
		networkPeers         []byte
		networkZones         []byte
		profiles             []byte
		projects             []byte
		storageBuckets       []byte
		storagePools         []byte
		storageVolumes       []byte
	)

	var inventoryAggregate inventory.InventoryAggregate

	err := row.Scan(
		&inventoryAggregate.Cluster,
		&servers,
		&images,
		&instances,
		&networks,
		&networkACLs,
		&networkForwards,
		&networkIntegrations,
		&networkLoadBalancers,
		&networkPeers,
		&networkZones,
		&profiles,
		&projects,
		&storageBuckets,
		&storagePools,
		&storageVolumes,
	)
	if err != nil {
		return inventory.InventoryAggregate{}, sqlite.MapErr(err)
	}

	err = json.Unmarshal(servers, &inventoryAggregate.Servers)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(images, &inventoryAggregate.Images)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(instances, &inventoryAggregate.Instances)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networks, &inventoryAggregate.Networks)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networkACLs, &inventoryAggregate.NetworkACLs)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networkForwards, &inventoryAggregate.NetworkForwards)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networkIntegrations, &inventoryAggregate.NetworkIntegrations)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networkLoadBalancers, &inventoryAggregate.NetworkLoadBalancers)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networkPeers, &inventoryAggregate.NetworkPeers)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(networkZones, &inventoryAggregate.NetworkZones)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(profiles, &inventoryAggregate.Profiles)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(projects, &inventoryAggregate.Projects)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(storageBuckets, &inventoryAggregate.StorageBuckets)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(storagePools, &inventoryAggregate.StoragePools)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	err = json.Unmarshal(storageVolumes, &inventoryAggregate.StorageVolumes)
	if err != nil {
		return inventory.InventoryAggregate{}, err
	}

	return inventoryAggregate, nil
}
