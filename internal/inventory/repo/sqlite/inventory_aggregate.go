package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/maps"
	"github.com/FuturFusion/operations-center/internal/ptr"
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

type inventoryResource struct {
	Kind        string
	ClusterName string
	ServerName  *string
	ProjectName *string
	ParentName  *string
	Name        string
	Object      []byte
}

func (r inventoryAggregate) GetAllWithFilter(ctx context.Context, filter inventory.InventoryAggregateFilter) (inventory.InventoryAggregates, error) {
	const sqlStmt = `
SELECT kind, cluster_name, server_name, project_name, parent_name, name, object FROM resources
WHERE true
%s
ORDER BY cluster_name
`

	var whereClause []string
	var args []any

	if filter.Kinds != nil {
		kinds := make([]string, 0, len(filter.Kinds))
		for i, kind := range filter.Kinds {
			kinds = append(kinds, fmt.Sprintf(":kind_%d", i))
			args = append(args, sql.Named(fmt.Sprintf("kind_%d", i), kind))
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND kind IN (%s)`, strings.Join(kinds, ", ")))
		args = append(args, sql.Named("cluster_name", filter.Cluster))
	}

	if filter.Cluster != nil {
		whereClause = append(whereClause, ` AND cluster_name = :cluster_name`)
		args = append(args, sql.Named("cluster_name", filter.Cluster))
	}

	if filter.Server != nil {
		serverCondition := `server_name = :server_name`
		if filter.ServerIncludeNull {
			serverCondition += ` OR server_name IS NULL`
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND (%s)`, serverCondition))
		args = append(args, sql.Named("server_name", filter.Server))
	}

	if filter.Project != nil {
		projectCondition := `project_name = :project_name`
		if filter.ProjectIncludeNull {
			projectCondition += ` OR project_name IS NULL`
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND (%s)`, projectCondition))
		args = append(args, sql.Named("project_name", filter.Project))
	}

	sqlStmtComplete := fmt.Sprintf(sqlStmt, strings.Join(whereClause, " "))

	rows, err := r.db.QueryContext(ctx, sqlStmtComplete, args...)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	inventoryAggregatesMap := make(map[string]inventory.InventoryAggregate)
	serversMap := make(map[string]map[string]struct{})
	for rows.Next() {
		var ok bool

		inventoryResource, err := scanInventoryResource(rows)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		inventoryAggregate, ok := inventoryAggregatesMap[inventoryResource.ClusterName]
		if !ok {
			inventoryAggregate.Cluster = inventoryResource.ClusterName
			serversMap[inventoryResource.ClusterName] = make(map[string]struct{})
		}

		if inventoryResource.ServerName != nil {
			serversMap[inventoryResource.ClusterName][ptr.From(inventoryResource.ServerName)] = struct{}{}
		}

		switch inventoryResource.Kind {
		case "image":
			if inventoryAggregate.Images == nil {
				images := make(map[string]map[string]incusapi.Image)
				inventoryAggregate.Images = images
			}

			_, ok = inventoryAggregate.Images[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.Images[ptr.From(inventoryResource.ProjectName)] = make(map[string]incusapi.Image)
			}

			var image incusapi.Image
			err := json.Unmarshal(inventoryResource.Object, &image)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.Images[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name] = image

		case "instance":
			if inventoryAggregate.Instances == nil {
				instances := make(map[string]map[string]map[string]incusapi.InstanceFull)
				inventoryAggregate.Instances = instances
			}

			_, ok = inventoryAggregate.Instances[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.Instances[ptr.From(inventoryResource.ProjectName)] = make(map[string]map[string]incusapi.InstanceFull)
			}

			_, ok = inventoryAggregate.Instances[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name]
			if !ok {
				inventoryAggregate.Instances[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name] = make(map[string]incusapi.InstanceFull)
			}

			var instance incusapi.InstanceFull
			err := json.Unmarshal(inventoryResource.Object, &instance)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.Instances[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name][ptr.From(inventoryResource.ServerName)] = instance

		case "network":
			if inventoryAggregate.Networks == nil {
				networks := make(map[string]map[string]incusapi.Network)
				inventoryAggregate.Networks = networks
			}

			_, ok = inventoryAggregate.Networks[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.Networks[ptr.From(inventoryResource.ProjectName)] = make(map[string]incusapi.Network)
			}

			var network incusapi.Network
			err := json.Unmarshal(inventoryResource.Object, &network)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.Networks[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name] = network

		case "network_acl":
			if inventoryAggregate.NetworkACLs == nil {
				networkACLs := make(map[string]map[string]incusapi.NetworkACL)
				inventoryAggregate.NetworkACLs = networkACLs
			}

			_, ok = inventoryAggregate.NetworkACLs[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.NetworkACLs[ptr.From(inventoryResource.ProjectName)] = make(map[string]incusapi.NetworkACL)
			}

			var networkACL incusapi.NetworkACL
			err := json.Unmarshal(inventoryResource.Object, &networkACL)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.NetworkACLs[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name] = networkACL

		case "network_forward":
			if inventoryAggregate.NetworkForwards == nil {
				networkForwards := make(map[string]map[string]incusapi.NetworkForward)
				inventoryAggregate.NetworkForwards = networkForwards
			}

			_, ok = inventoryAggregate.NetworkForwards[ptr.From(inventoryResource.ParentName)]
			if !ok {
				inventoryAggregate.NetworkForwards[ptr.From(inventoryResource.ParentName)] = make(map[string]incusapi.NetworkForward)
			}

			var networkForward incusapi.NetworkForward
			err := json.Unmarshal(inventoryResource.Object, &networkForward)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.NetworkForwards[ptr.From(inventoryResource.ParentName)][inventoryResource.Name] = networkForward

		case "network_integration":
			if inventoryAggregate.NetworkIntegrations == nil {
				networkIntegrations := make(map[string]incusapi.NetworkIntegration)
				inventoryAggregate.NetworkIntegrations = networkIntegrations
			}

			var networkIntegration incusapi.NetworkIntegration
			err := json.Unmarshal(inventoryResource.Object, &networkIntegration)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.NetworkIntegrations[inventoryResource.Name] = networkIntegration

		case "network_load_balancer":
			if inventoryAggregate.NetworkLoadBalancers == nil {
				networkLoadBalancers := make(map[string]map[string]incusapi.NetworkLoadBalancer)
				inventoryAggregate.NetworkLoadBalancers = networkLoadBalancers
			}

			_, ok = inventoryAggregate.NetworkLoadBalancers[ptr.From(inventoryResource.ParentName)]
			if !ok {
				inventoryAggregate.NetworkLoadBalancers[ptr.From(inventoryResource.ParentName)] = make(map[string]incusapi.NetworkLoadBalancer)
			}

			var networkLoadBalancer incusapi.NetworkLoadBalancer
			err := json.Unmarshal(inventoryResource.Object, &networkLoadBalancer)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.NetworkLoadBalancers[ptr.From(inventoryResource.ParentName)][inventoryResource.Name] = networkLoadBalancer

		case "network_peer":
			if inventoryAggregate.NetworkPeers == nil {
				networkPeers := make(map[string]map[string]incusapi.NetworkPeer)
				inventoryAggregate.NetworkPeers = networkPeers
			}

			_, ok = inventoryAggregate.NetworkPeers[ptr.From(inventoryResource.ParentName)]
			if !ok {
				inventoryAggregate.NetworkPeers[ptr.From(inventoryResource.ParentName)] = make(map[string]incusapi.NetworkPeer)
			}

			var networkPeer incusapi.NetworkPeer
			err := json.Unmarshal(inventoryResource.Object, &networkPeer)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.NetworkPeers[ptr.From(inventoryResource.ParentName)][inventoryResource.Name] = networkPeer

		case "network_zone":
			if inventoryAggregate.NetworkZones == nil {
				networkZones := make(map[string]map[string]incusapi.NetworkZone)
				inventoryAggregate.NetworkZones = networkZones
			}

			_, ok = inventoryAggregate.NetworkZones[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.NetworkZones[ptr.From(inventoryResource.ProjectName)] = make(map[string]incusapi.NetworkZone)
			}

			var networkZone incusapi.NetworkZone
			err := json.Unmarshal(inventoryResource.Object, &networkZone)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.NetworkZones[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name] = networkZone

		case "profile":
			if inventoryAggregate.Profiles == nil {
				profiles := make(map[string]map[string]incusapi.Profile)
				inventoryAggregate.Profiles = profiles
			}

			_, ok = inventoryAggregate.Profiles[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.Profiles[ptr.From(inventoryResource.ProjectName)] = make(map[string]incusapi.Profile)
			}

			var profile incusapi.Profile
			err := json.Unmarshal(inventoryResource.Object, &profile)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.Profiles[ptr.From(inventoryResource.ProjectName)][inventoryResource.Name] = profile

		case "project":
			if inventoryAggregate.Projects == nil {
				projects := make(map[string]incusapi.Project)
				inventoryAggregate.Projects = projects
			}

			var project incusapi.Project
			err := json.Unmarshal(inventoryResource.Object, &project)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.Projects[inventoryResource.Name] = project

		case "storage_bucket":
			if inventoryAggregate.StorageBuckets == nil {
				storageBuckets := make(map[string]map[string]map[string]map[string]incusapi.StorageBucket)
				inventoryAggregate.StorageBuckets = storageBuckets
			}

			_, ok = inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)] = make(map[string]map[string]map[string]incusapi.StorageBucket)
			}

			_, ok = inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)]
			if !ok {
				inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)] = make(map[string]map[string]incusapi.StorageBucket)
			}

			_, ok = inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)][inventoryResource.Name]
			if !ok {
				inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)][inventoryResource.Name] = make(map[string]incusapi.StorageBucket)
			}

			var storageBucket incusapi.StorageBucket
			err := json.Unmarshal(inventoryResource.Object, &storageBucket)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.StorageBuckets[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)][inventoryResource.Name][ptr.From(inventoryResource.ServerName)] = storageBucket

		case "storage_pool":
			if inventoryAggregate.StoragePools == nil {
				storagePools := make(map[string]incusapi.StoragePool)
				inventoryAggregate.StoragePools = storagePools
			}

			var storagePool incusapi.StoragePool
			err := json.Unmarshal(inventoryResource.Object, &storagePool)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.StoragePools[inventoryResource.Name] = storagePool

		case "storage_volume":
			if inventoryAggregate.StorageVolumes == nil {
				storageVolumes := make(map[string]map[string]map[string]map[string]incusapi.StorageVolume)
				inventoryAggregate.StorageVolumes = storageVolumes
			}

			_, ok = inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)]
			if !ok {
				inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)] = make(map[string]map[string]map[string]incusapi.StorageVolume)
			}

			_, ok = inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)]
			if !ok {
				inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)] = make(map[string]map[string]incusapi.StorageVolume)
			}

			_, ok = inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)][inventoryResource.Name]
			if !ok {
				inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)][inventoryResource.Name] = make(map[string]incusapi.StorageVolume)
			}

			var storageVolume incusapi.StorageVolume
			err := json.Unmarshal(inventoryResource.Object, &storageVolume)
			if err != nil {
				return nil, err
			}

			inventoryAggregate.StorageVolumes[ptr.From(inventoryResource.ProjectName)][ptr.From(inventoryResource.ParentName)][inventoryResource.Name][ptr.From(inventoryResource.ServerName)] = storageVolume

		default:
			return nil, fmt.Errorf("Unknown inventory resource kind %q", inventoryResource.Kind)
		}

		inventoryAggregatesMap[inventoryResource.ClusterName] = inventoryAggregate
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	inventoryAggregates := make(inventory.InventoryAggregates, 0, len(inventoryAggregatesMap))
	for _, inventoryAgregate := range maps.OrderedByKey(inventoryAggregatesMap) {
		servers := make([]string, 0, len(serversMap[inventoryAgregate.Cluster]))
		for server := range maps.OrderedByKey(serversMap[inventoryAgregate.Cluster]) {
			servers = append(servers, server)
		}

		inventoryAgregate.Servers = servers
		inventoryAggregates = append(inventoryAggregates, inventoryAgregate)
	}

	return inventoryAggregates, nil
}

func scanInventoryResource(row interface{ Scan(dest ...any) error }) (inventoryResource, error) {
	var resourceItem inventoryResource

	err := row.Scan(
		&resourceItem.Kind,
		&resourceItem.ClusterName,
		&resourceItem.ServerName,
		&resourceItem.ProjectName,
		&resourceItem.ParentName,
		&resourceItem.Name,
		&resourceItem.Object,
	)
	if err != nil {
		return inventoryResource{}, sqlite.MapErr(err)
	}

	return resourceItem, nil
}
