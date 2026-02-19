package api

import (
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

type queryHandler struct {
	service inventory.InventoryAggregateService
}

func registerInventoryQueryHandler(router Router, authorizer *authz.Authorizer, service inventory.InventoryAggregateService) {
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

	result := mapInventoryAggregateToAPITypes(inventoryAggregates)

	return response.SyncResponse(true, result)
}
