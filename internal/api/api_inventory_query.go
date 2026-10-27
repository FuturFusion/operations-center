package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
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

// swagger:operation GET /1.0/inventory/query inventory query_get
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
//	    x-example: cluster
//	  - in: query
//	    name: project
//	    description: Project name
//	    type: string
//	    x-example: default
//	  - in: query
//	    name: server
//	    description: Server name
//	    type: string
//	    x-example: server
//	  - in: query
//	    name: kind
//	    description: Kind of resources to be queried
//	    type: array
//	    items:
//	      type: string
//	    collectionFormat: multi
//	    x-example: project
//	  - in: query
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    x-example: name == "value"
//	responses:
//	  "200":
//	    $ref: "#/responses/InventoryAggregatesResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *queryHandler) querysGet(r *http.Request) response.Response {
	filter, err := api.InventoryAggregateFilterFromURLValues(r.URL.Query())
	if err != nil {
		return response.SmartError(err)
	}

	// FIXME: Should we require a non empty filter with recursion?
	inventoryAggregates, err := i.service.GetAllWithFilter(r.Context(), filter)
	if err != nil {
		return response.SmartError(err)
	}

	result := mapInventoryAggregateToAPITypes(inventoryAggregates)

	return response.SyncResponse(true, result)
}
