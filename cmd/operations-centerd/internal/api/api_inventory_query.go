package api

import (
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type queryHandler struct {
	service inventory.InventoryAggregateService
}

func registerInventoryQueryHandler(router *http.ServeMux, service inventory.InventoryAggregateService) {
	handler := &queryHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.querysGet))
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

	if r.URL.Query().Get("cluster") != "" {
		filter.Cluster = ptr.To(r.URL.Query().Get("cluster"))
	}

	if r.URL.Query().Get("server") != "" {
		filter.Server = ptr.To(r.URL.Query().Get("server"))
	}

	if r.URL.Query().Get("server_include_null") != "" {
		serverIncludeNull, err := strconv.ParseBool(r.URL.Query().Get("server_include_null"))
		if err != nil {
			return response.SmartError(err)
		}

		filter.ServerIncludeNull = serverIncludeNull
	}

	if r.URL.Query().Get("project") != "" {
		filter.Project = ptr.To(r.URL.Query().Get("project"))
	}

	if r.URL.Query().Get("project_include_null") != "" {
		projectIncludeNull, err := strconv.ParseBool(r.URL.Query().Get("project_include_null"))
		if err != nil {
			return response.SmartError(err)
		}

		filter.ProjectIncludeNull = projectIncludeNull
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
		result = append(result, api.InventoryAggregate{
			Cluster:              inventoryAggregate.Cluster,
			Servers:              inventoryAggregate.Servers,
			Images:               inventoryAggregate.Images,
			Instances:            inventoryAggregate.Instances,
			Networks:             inventoryAggregate.Networks,
			NetworkACLs:          inventoryAggregate.NetworkACLs,
			NetworkForwards:      inventoryAggregate.NetworkForwards,
			NetworkIntegrations:  inventoryAggregate.NetworkIntegrations,
			NetworkLoadBalancers: inventoryAggregate.NetworkLoadBalancers,
			NetworkPeers:         inventoryAggregate.NetworkPeers,
			NetworkZones:         inventoryAggregate.NetworkZones,
			Profiles:             inventoryAggregate.Profiles,
			Projects:             inventoryAggregate.Projects,
			StorageBuckets:       inventoryAggregate.StorageBuckets,
			StoragePools:         inventoryAggregate.StoragePools,
			StorageVolumes:       inventoryAggregate.StorageVolumes,
		})
	}

	return response.SyncResponse(true, result)
}
