package api

import (
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type queryHandler struct {
	service inventory.InventoryAggregateService
}

func registerInventoryQueryHandler(router Router, authorizer authz.Authorizer, service inventory.InventoryAggregateService) {
	handler := &queryHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.querysGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
}

// swagger:operation GET /1.0/inventory/query query query_get
//
//	Query resources from inventory
//
//	Returns structured resources from the inventory.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: cluster
//	    description: Cluster name
//	    type: string
//	    example: cluster
//	  - in: query
//	    name: project
//	    description: Project name
//	    type: string
//	    example: default
//	  - in: query
//	    name: server
//	    description: Server name
//	    type: string
//	    example: server
//	  - in: query
//	    name: kind
//	    description: Kind of resources to be queried
//	    type: array
//	    example: [ "project", "profile" ]
//	  - in: query
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API query
//	    schema:
//	      type: object
//	      description: Sync response
//	      properties:
//	        type:
//	          type: string
//	          description: Response type
//	          example: sync
//	        status:
//	          type: string
//	          description: Status description
//	          example: Success
//	        status_code:
//	          type: integer
//	          description: Status code
//	          example: 200
//	        metadata:
//	          type: array
//	          description: List of aggregated resources from the inventory
//	          items:
//	            $ref: "#/definitions/inventory_aggregate"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *queryHandler) querysGet(r *http.Request) response.Response {
	var filter inventory.InventoryAggregateFilter

	kinds, ok := r.URL.Query()["kind"]
	if ok {
		filter.Kinds = kinds
	}

	clusters, ok := r.URL.Query()["cluster"]
	if ok {
		filter.Clusters = clusters
	}

	servers, ok := r.URL.Query()["server"]
	if ok {
		filter.Servers = servers
	}

	if r.URL.Query().Get("server_include_null") != "" {
		serverIncludeNull, err := strconv.ParseBool(r.URL.Query().Get("server_include_null"))
		if err != nil {
			return response.SmartError(err)
		}

		filter.ServerIncludeNull = serverIncludeNull
	}

	projects, ok := r.URL.Query()["project"]
	if ok {
		filter.Projects = projects
	}

	if r.URL.Query().Get("project_include_null") != "" {
		projectIncludeNull, err := strconv.ParseBool(r.URL.Query().Get("project_include_null"))
		if err != nil {
			return response.SmartError(err)
		}

		filter.ProjectIncludeNull = projectIncludeNull
	}

	parents, ok := r.URL.Query()["parent"]
	if ok {
		filter.Parents = parents
	}

	if r.URL.Query().Get("parent_include_null") != "" {
		parentIncludeNull, err := strconv.ParseBool(r.URL.Query().Get("parent_include_null"))
		if err != nil {
			return response.SmartError(err)
		}

		filter.ProjectIncludeNull = parentIncludeNull
	}

	if r.URL.Query().Get("filter") != "" {
		filter.Expression = ptr.To(r.URL.Query().Get("filter"))
	}

	// FIXME: Should we require a non empty filter with recursion?
	inventoryAggregates, err := i.service.GetAllWithFilter(r.Context(), filter)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]api.InventoryAggregate, 0, len(inventoryAggregates))
	for _, inventoryAggregate := range inventoryAggregates {
		resultItem := api.InventoryAggregate{
			Cluster: inventoryAggregate.Cluster,
			Servers: inventoryAggregate.Servers,
		}

		for _, image := range inventoryAggregate.Images {
			resultItem.Images = append(resultItem.Images, api.Image{
				Cluster:     image.Cluster,
				ProjectName: image.ProjectName,
				Name:        image.Name,
				Object:      image.Object,
			})
		}

		for _, instance := range inventoryAggregate.Instances {
			resultItem.Instances = append(resultItem.Instances, api.Instance{
				Cluster:     instance.Cluster,
				Server:      instance.Server,
				ProjectName: instance.ProjectName,
				Name:        instance.Name,
				Object:      instance.Object,
			})
		}

		for _, network := range inventoryAggregate.Networks {
			resultItem.Networks = append(resultItem.Networks, api.Network{
				Cluster:     network.Cluster,
				ProjectName: network.ProjectName,
				Name:        network.Name,
				Object:      network.Object,
			})
		}

		for _, networkACL := range inventoryAggregate.NetworkACLs {
			resultItem.NetworkACLs = append(resultItem.NetworkACLs, api.NetworkACL{
				Cluster:     networkACL.Cluster,
				ProjectName: networkACL.ProjectName,
				Name:        networkACL.Name,
				Object:      networkACL.Object,
			})
		}

		for _, networkForward := range inventoryAggregate.NetworkForwards {
			resultItem.NetworkForwards = append(resultItem.NetworkForwards, api.NetworkForward{
				Cluster:     networkForward.Cluster,
				NetworkName: networkForward.NetworkName,
				Name:        networkForward.Name,
				Object:      networkForward.Object,
			})
		}

		for _, networkIntegration := range inventoryAggregate.NetworkIntegrations {
			resultItem.NetworkIntegrations = append(resultItem.NetworkIntegrations, api.NetworkIntegration{
				Cluster: networkIntegration.Cluster,
				Name:    networkIntegration.Name,
				Object:  networkIntegration.Object,
			})
		}

		for _, networkLoadBalancer := range inventoryAggregate.NetworkLoadBalancers {
			resultItem.NetworkLoadBalancers = append(resultItem.NetworkLoadBalancers, api.NetworkLoadBalancer{
				Cluster:     networkLoadBalancer.Cluster,
				NetworkName: networkLoadBalancer.NetworkName,
				Name:        networkLoadBalancer.Name,
				Object:      networkLoadBalancer.Object,
			})
		}

		for _, networkPeer := range inventoryAggregate.NetworkPeers {
			resultItem.NetworkPeers = append(resultItem.NetworkPeers, api.NetworkPeer{
				Cluster:     networkPeer.Cluster,
				NetworkName: networkPeer.NetworkName,
				Name:        networkPeer.Name,
				Object:      networkPeer.Object,
			})
		}

		for _, networkZone := range inventoryAggregate.NetworkZones {
			resultItem.NetworkZones = append(resultItem.NetworkZones, api.NetworkZone{
				Cluster:     networkZone.Cluster,
				ProjectName: networkZone.ProjectName,
				Name:        networkZone.Name,
				Object:      networkZone.Object,
			})
		}

		for _, profile := range inventoryAggregate.Profiles {
			resultItem.Profiles = append(resultItem.Profiles, api.Profile{
				Cluster:     profile.Cluster,
				ProjectName: profile.ProjectName,
				Name:        profile.Name,
				Object:      profile.Object,
			})
		}

		for _, project := range inventoryAggregate.Projects {
			resultItem.Projects = append(resultItem.Projects, api.Project{
				Cluster: project.Cluster,
				Name:    project.Name,
				Object:  project.Object,
			})
		}

		for _, storageBucket := range inventoryAggregate.StorageBuckets {
			resultItem.StorageBuckets = append(resultItem.StorageBuckets, api.StorageBucket{
				Cluster:         storageBucket.Cluster,
				Server:          storageBucket.Server,
				ProjectName:     storageBucket.ProjectName,
				StoragePoolName: storageBucket.StoragePoolName,
				Name:            storageBucket.Name,
				Object:          storageBucket.Object,
			})
		}

		for _, storagePool := range inventoryAggregate.StoragePools {
			resultItem.StoragePools = append(resultItem.StoragePools, api.StoragePool{
				Cluster: storagePool.Cluster,
				Name:    storagePool.Name,
				Object:  storagePool.Object,
			})
		}

		for _, storageVolume := range inventoryAggregate.StorageVolumes {
			resultItem.StorageVolumes = append(resultItem.StorageVolumes, api.StorageVolume{
				Cluster:         storageVolume.Cluster,
				Server:          storageVolume.Server,
				ProjectName:     storageVolume.ProjectName,
				StoragePoolName: storageVolume.StoragePoolName,
				Name:            storageVolume.Name,
				Object:          storageVolume.Object,
			})
		}

		result = append(result, resultItem)
	}

	return response.SyncResponse(true, result)
}
