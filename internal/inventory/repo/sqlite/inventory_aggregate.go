package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
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

type InventoryResource struct {
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
ORDER BY cluster_name, project_name, parent_name, name, server_name
`
	var err error
	var filterExpression *vm.Program
	var whereClause []string
	var args []any

	type Env struct {
		Kind        string         `expr:"kind"`
		ClusterName string         `expr:"cluster_name"`
		ServerName  string         `expr:"server_name"`
		ProjectName string         `expr:"project_name"`
		ParentName  string         `expr:"parent_name"`
		Name        string         `expr:"name"`
		Object      map[string]any `expr:"object"`
	}

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(Env{})}...)
		if err != nil {
			return nil, err
		}
	}

	if filter.Kinds != nil {
		kinds := make([]string, 0, len(filter.Kinds))
		for i, kind := range filter.Kinds {
			kinds = append(kinds, fmt.Sprintf(":kind_%d", i))
			args = append(args, sql.Named(fmt.Sprintf("kind_%d", i), kind))
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND kind IN (%s)`, strings.Join(kinds, ", ")))
	}

	if len(filter.Clusters) > 0 {
		clusters := make([]string, 0, len(filter.Clusters))
		for i, cluster := range filter.Clusters {
			clusters = append(clusters, fmt.Sprintf(`:cluster_name_%d`, i))
			args = append(args, sql.Named(fmt.Sprintf("cluster_name_%d", i), cluster))
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND cluster_name IN (%s)`, strings.Join(clusters, ", ")))
	}

	if len(filter.Servers) > 0 {
		servers := make([]string, 0, len(filter.Servers))
		for i, server := range filter.Servers {
			servers = append(servers, fmt.Sprintf(`:server_name_%d`, i))
			args = append(args, sql.Named(fmt.Sprintf("server_name_%d", i), server))
		}

		serverIsNil := ``
		if filter.ServerIncludeNull {
			serverIsNil += ` OR server_name IS NULL`
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND (server_name IN (%s)%s)`, strings.Join(servers, ", "), serverIsNil))
	}

	if len(filter.Projects) > 0 {
		projects := make([]string, 0, len(filter.Projects))
		for i, project := range filter.Projects {
			projects = append(projects, fmt.Sprintf(`:project_name_%d`, i))
			args = append(args, sql.Named(fmt.Sprintf("project_name_%d", i), project))
		}

		projectIsNil := ``
		if filter.ProjectIncludeNull {
			projectIsNil += ` OR project_name IS NULL`
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND (project_name IN (%s)%s)`, strings.Join(projects, ", "), projectIsNil))
	}

	if len(filter.Parents) > 0 {
		parents := make([]string, 0, len(filter.Parents))
		for i, parent := range filter.Parents {
			parents = append(parents, fmt.Sprintf(`:parent_name_%d`, i))
			args = append(args, sql.Named(fmt.Sprintf("parent_name_%d", i), parent))
		}

		parentIsNil := ``
		if filter.ParentIncludeNull {
			parentIsNil += ` OR parent_name IS NULL`
		}

		whereClause = append(whereClause, fmt.Sprintf(` AND (parent_name IN (%s)%s)`, strings.Join(parents, ", "), parentIsNil))
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

		if filter.Expression != nil {
			object := map[string]any{}
			err = json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			output, err := expr.Run(filterExpression, Env{
				Kind:        inventoryResource.Kind,
				ClusterName: inventoryResource.ClusterName,
				ServerName:  ptr.From(inventoryResource.ServerName),
				ProjectName: ptr.From(inventoryResource.ProjectName),
				ParentName:  ptr.From(inventoryResource.ParentName),
				Name:        inventoryResource.Name,
				Object:      object,
			})
			if err != nil {
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Include expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if !result {
				continue
			}
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
			var object incusapi.Image
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			image := inventory.Image{
				Cluster:     inventoryResource.ClusterName,
				ProjectName: ptr.From(inventoryResource.ProjectName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusImageWrapper{
					Image: object,
				},
			}

			inventoryAggregate.Images = append(inventoryAggregate.Images, image)

		case "instance":
			var object incusapi.InstanceFull
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			instance := inventory.Instance{
				Cluster:     inventoryResource.ClusterName,
				ProjectName: ptr.From(inventoryResource.ProjectName),
				Server:      ptr.From(inventoryResource.ServerName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusInstanceFullWrapper{
					InstanceFull: object,
				},
			}

			inventoryAggregate.Instances = append(inventoryAggregate.Instances, instance)

		case "network":
			var object incusapi.Network
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			network := inventory.Network{
				Cluster:     inventoryResource.ClusterName,
				ProjectName: ptr.From(inventoryResource.ProjectName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusNetworkWrapper{
					Network: object,
				},
			}

			inventoryAggregate.Networks = append(inventoryAggregate.Networks, network)

		case "network_acl":
			var object incusapi.NetworkACL
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			networkACL := inventory.NetworkACL{
				Cluster:     inventoryResource.ClusterName,
				ProjectName: ptr.From(inventoryResource.ProjectName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusNetworkACLWrapper{
					NetworkACL: object,
				},
			}

			inventoryAggregate.NetworkACLs = append(inventoryAggregate.NetworkACLs, networkACL)

		case "network_forward":
			var object incusapi.NetworkForward
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			networkForward := inventory.NetworkForward{
				Cluster:     inventoryResource.ClusterName,
				NetworkName: ptr.From(inventoryResource.ParentName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusNetworkForwardWrapper{
					NetworkForward: object,
				},
			}

			inventoryAggregate.NetworkForwards = append(inventoryAggregate.NetworkForwards, networkForward)

		case "network_integration":
			var object incusapi.NetworkIntegration
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			networkIntegration := inventory.NetworkIntegration{
				Cluster: inventoryResource.ClusterName,
				Name:    inventoryResource.Name,
				Object: inventory.IncusNetworkIntegrationWrapper{
					NetworkIntegration: object,
				},
			}

			inventoryAggregate.NetworkIntegrations = append(inventoryAggregate.NetworkIntegrations, networkIntegration)

		case "network_load_balancer":
			var object incusapi.NetworkLoadBalancer
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			networkLoadBalancer := inventory.NetworkLoadBalancer{
				Cluster:     inventoryResource.ClusterName,
				NetworkName: ptr.From(inventoryResource.ParentName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusNetworkLoadBalancerWrapper{
					NetworkLoadBalancer: object,
				},
			}

			inventoryAggregate.NetworkLoadBalancers = append(inventoryAggregate.NetworkLoadBalancers, networkLoadBalancer)

		case "network_peer":
			var object incusapi.NetworkPeer
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			networkPeer := inventory.NetworkPeer{
				Cluster:     inventoryResource.ClusterName,
				NetworkName: ptr.From(inventoryResource.ParentName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusNetworkPeerWrapper{
					NetworkPeer: object,
				},
			}

			inventoryAggregate.NetworkPeers = append(inventoryAggregate.NetworkPeers, networkPeer)

		case "network_zone":
			var object incusapi.NetworkZone
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			networkZone := inventory.NetworkZone{
				Cluster:     inventoryResource.ClusterName,
				ProjectName: ptr.From(inventoryResource.ProjectName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusNetworkZoneWrapper{
					NetworkZone: object,
				},
			}

			inventoryAggregate.NetworkZones = append(inventoryAggregate.NetworkZones, networkZone)

		case "profile":
			var object incusapi.Profile
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			profile := inventory.Profile{
				Cluster:     inventoryResource.ClusterName,
				ProjectName: ptr.From(inventoryResource.ProjectName),
				Name:        inventoryResource.Name,
				Object: inventory.IncusProfileWrapper{
					Profile: object,
				},
			}

			inventoryAggregate.Profiles = append(inventoryAggregate.Profiles, profile)

		case "project":
			var object incusapi.Project
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			project := inventory.Project{
				Cluster: inventoryResource.ClusterName,
				Name:    inventoryResource.Name,
				Object: inventory.IncusProjectWrapper{
					Project: object,
				},
			}

			inventoryAggregate.Projects = append(inventoryAggregate.Projects, project)

		case "storage_bucket":
			var object incusapi.StorageBucketFull
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			storageBucket := inventory.StorageBucket{
				Cluster:         inventoryResource.ClusterName,
				ProjectName:     ptr.From(inventoryResource.ProjectName),
				StoragePoolName: ptr.From(inventoryResource.ParentName),
				Server:          ptr.From(inventoryResource.ServerName),
				Name:            inventoryResource.Name,
				Object: inventory.IncusStorageBucketFullWrapper{
					StorageBucketFull: object,
				},
			}

			inventoryAggregate.StorageBuckets = append(inventoryAggregate.StorageBuckets, storageBucket)

		case "storage_pool":
			var object incusapi.StoragePool
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			storagePool := inventory.StoragePool{
				Cluster: inventoryResource.ClusterName,
				Name:    inventoryResource.Name,
				Object: inventory.IncusStoragePoolWrapper{
					StoragePool: object,
				},
			}

			inventoryAggregate.StoragePools = append(inventoryAggregate.StoragePools, storagePool)

		case "storage_volume":
			var object incusapi.StorageVolumeFull
			err := json.Unmarshal(inventoryResource.Object, &object)
			if err != nil {
				return nil, err
			}

			storageVolume := inventory.StorageVolume{
				Cluster:         inventoryResource.ClusterName,
				ProjectName:     ptr.From(inventoryResource.ProjectName),
				StoragePoolName: ptr.From(inventoryResource.ParentName),
				Server:          inventoryResource.ServerName,
				Name:            inventoryResource.Name,
				Object: inventory.IncusStorageVolumeFullWrapper{
					StorageVolumeFull: object,
				},
			}

			inventoryAggregate.StorageVolumes = append(inventoryAggregate.StorageVolumes, storageVolume)

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

func scanInventoryResource(row interface{ Scan(dest ...any) error }) (InventoryResource, error) {
	var resourceItem InventoryResource

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
		return InventoryResource{}, sqlite.MapErr(err)
	}

	return resourceItem, nil
}
