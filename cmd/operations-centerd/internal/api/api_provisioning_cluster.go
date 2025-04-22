package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type clusterHandler struct {
	service provisioning.ClusterService
}

func registerProvisioningClusterHandler(router Router, authorizer authz.Authorizer, service provisioning.ClusterService) {
	handler := &clusterHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.clustersGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.clustersPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{name}", response.With(handler.clusterGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.clusterPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.clusterDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}", response.With(handler.clusterPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/resync-inventory", response.With(handler.clusterResyncInventoryPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/provisioning/clusters clusters clusters_get
//
//	Get the clusters
//
//	Returns a list of clusters (URLs).
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API clusters
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
//	          description: List of clusters
//                items:
//                  type: string
//                example: |-
//                  [
//                    "/1.0/provisioning/clusters/one",
//                    "/1.0/provisioning/clusters/two"
//                  ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/clusters?recursion=1 clusters clusters_get_recursion
//
//	Get the clusters
//
//	Returns a list of clusters (structs).
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API clusters
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
//	          description: List of clusters
//	          items:
//	            $ref: "#/definitions/Cluster"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clustersGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	var filter provisioning.ClusterFilter

	if r.URL.Query().Get("filter") != "" {
		filter.Expression = ptr.To(r.URL.Query().Get("filter"))
	}

	if recursion == 1 {
		clusters, err := c.service.GetAllWithFilter(r.Context(), filter)
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Cluster, 0, len(clusters))
		for _, cluster := range clusters {
			result = append(result, api.Cluster{
				Name:          cluster.Name,
				ConnectionURL: cluster.ConnectionURL,
				LastUpdated:   cluster.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	clusterNames, err := c.service.GetAllNamesWithFilter(r.Context(), filter)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(clusterNames))
	for _, name := range clusterNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/clusters/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/clusters clusters clusters_post
//
//	Add a cluster
//
//	Creates a new cluster.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster
//	    description: Cluster configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ClusterPost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clustersPost(r *http.Request) response.Response {
	var cluster api.ClusterPost

	// Decode into the new cluster.
	err := json.NewDecoder(r.Body).Decode(&cluster)
	if err != nil {
		return response.BadRequest(err)
	}

	_, err = c.service.Create(r.Context(), provisioning.Cluster{
		Name:          cluster.Name,
		ConnectionURL: cluster.ConnectionURL,
		ServerNames:   cluster.ServerNames,
		LastUpdated:   cluster.LastUpdated,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating cluster: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/clusters/"+cluster.Name)
}

// swagger:operation GET /1.0/provisioning/clusters/{name} clusters cluster_get
//
//	Get the cluster
//
//	Gets a specific cluster.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Cluster
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
//	          $ref: "#/definitions/Cluster"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	cluster, err := c.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Cluster{
			Name:          cluster.Name,
			ConnectionURL: cluster.ConnectionURL,
			LastUpdated:   cluster.LastUpdated,
		},
		cluster,
	)
}

// swagger:operation PUT /1.0/provisioning/clusters/{name} clusters cluster_put
//
//	Update the cluster
//
//	Updates the cluster definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster
//	    description: Cluster definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/Cluster"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterPut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var cluster api.Cluster

	err := json.NewDecoder(r.Body).Decode(&cluster)
	if err != nil {
		return response.BadRequest(err)
	}

	ctx, trans := transaction.Begin(r.Context())
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			response.SmartError(fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err))
		}
	}()

	currentCluster, err := c.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get cluster %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentCluster)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = c.service.Update(ctx, provisioning.Cluster{
		Name:          cluster.Name,
		ConnectionURL: cluster.ConnectionURL,
		LastUpdated:   cluster.LastUpdated,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating cluster %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/clusters/"+name)
}

// swagger:operation DELETE /1.0/provisioning/clusters/{name} clusters cluster_delete
//
//	Delete the cluster
//
//	Removes the cluster.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := c.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/clusters/{name} clusters cluster_post
//
//	Rename the cluster
//
//	Renames the cluster.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster
//	    description: Cluster definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/Cluster"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	var cluster api.Cluster

	err := json.NewDecoder(r.Body).Decode(&cluster)
	if err != nil {
		return response.BadRequest(err)
	}

	ctx, trans := transaction.Begin(r.Context())
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			response.SmartError(fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err))
		}
	}()

	currentCluster, err := c.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get cluster %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentCluster)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = c.service.Rename(ctx, name, cluster.Name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed renaming cluster %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/clusters/"+cluster.Name)
}

// swagger:operation POST /1.0/provisioning/clusters/{name}/resync-inventory clusters cluster_resync_inventory_post
//
//	Resync the cluster's inventory
//
//	Resync the inventory of a specific cluster.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Empty response
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
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterResyncInventoryPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := c.service.ResyncInventoryByName(r.Context(), name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to resync inventory for cluster: %w", err))
	}

	return response.EmptySyncResponse
}
