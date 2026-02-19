package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

type clusterHandler struct {
	service            provisioning.ClusterService
	clusterTemplateSvc provisioning.ClusterTemplateService
}

func registerProvisioningClusterHandler(router Router, authorizer *authz.Authorizer, service provisioning.ClusterService, clusterTemplateSvc provisioning.ClusterTemplateService) {
	handler := &clusterHandler{
		service:            service,
		clusterTemplateSvc: clusterTemplateSvc,
	}

	router.HandleFunc("GET /{$}", response.With(handler.clustersGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.clustersPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{name}", response.With(handler.clusterGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.clusterPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.clusterDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}", response.With(handler.clusterPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/:resync-inventory", response.With(handler.clusterResyncInventoryPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("PUT /{name}/certificate", response.With(handler.clusterCertificatePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /{clusterName}/artifacts", response.With(handler.clusterArtifactsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{clusterName}/artifacts/{artifactName}", response.With(handler.clusterArtifactGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{clusterName}/artifacts/{artifactName}/{filename}", response.With(handler.clusterArtifactFileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
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
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/provisioning/clusters/one",
//	              "/1.0/provisioning/clusters/two"
//	            ]
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
				Name: cluster.Name,
				ClusterPut: api.ClusterPut{
					ConnectionURL: cluster.ConnectionURL,
					Channel:       cluster.Channel,
				},
				Certificate: ptr.From(cluster.Certificate),
				Fingerprint: cluster.Fingerprint,
				Status:      cluster.Status,
				LastUpdated: cluster.LastUpdated,
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

	if cluster.ClusterTemplate != "" {
		cluster.ServicesConfig, cluster.ApplicationSeedConfig, err = c.clusterTemplateSvc.Apply(r.Context(), cluster.ClusterTemplate, cluster.ClusterTemplateVariableValues)
		if err != nil {
			return response.SmartError(fmt.Errorf("Failed creating cluster from template %q: %w", cluster.ClusterTemplate, err))
		}
	}

	_, err = c.service.Create(r.Context(), provisioning.Cluster{
		Name:                  cluster.Name,
		ConnectionURL:         cluster.ConnectionURL,
		ServerNames:           cluster.ServerNames,
		ServerType:            cluster.ServerType,
		ServicesConfig:        cluster.ServicesConfig,
		ApplicationSeedConfig: cluster.ApplicationSeedConfig,
		Channel:               cluster.Channel,
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
			Name: cluster.Name,
			ClusterPut: api.ClusterPut{
				ConnectionURL: cluster.ConnectionURL,
				Channel:       cluster.Channel,
			},
			Certificate: ptr.From(cluster.Certificate),
			Fingerprint: cluster.Fingerprint,
			Status:      cluster.Status,
			LastUpdated: cluster.LastUpdated,
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

	var cluster api.ClusterPut

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

	currentCluster.ConnectionURL = cluster.ConnectionURL
	currentCluster.Channel = cluster.Channel

	err = c.service.Update(ctx, *currentCluster)
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
//	parameters:
//	  - in: query
//	    name: mode
//	    description: |
//	      Delete mode, one of "normal", "force" or "factory-reset", defaults to "normal".
//
//	        - normal: cluster record is only removed from operations center if it is in state pending or unknown and there are no servers referencing the cluster.
//	        - force: cluster and server records including all associated inventory information is removed from operations center, does not do any change to the cluster it self.
//	        - factory-reset: everything from "force" and additionally a factory reset is performed on every server, that is part of the cluster.
//	    type: string
//	    example: normal
//	  - in: query
//	    name: token
//	    description: Token UUID
//	    type: string
//	    example: f1710b8e-cd77-4336-897a-96ff0e0ed529
//	  - in: query
//	    name: tokenSeedName
//	    description: Token seed name for the given token.
//	    type: string
//	    example: token-seed-name
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
	mode := r.URL.Query().Get("mode")

	deleteMode := api.ClusterDeleteMode(mode)
	_, ok := api.ClusterDeleteModes[deleteMode]
	if !ok {
		deleteMode = api.ClusterDeleteModeNormal
	}

	if deleteMode == api.ClusterDeleteModeFactoryReset {
		var tokenID *uuid.UUID
		var tokenSeedName *string

		if r.URL.Query().Get("tokenSeedName") != "" {
			tokenSeedName = ptr.To(r.URL.Query().Get("tokenSeedName"))
		}

		if r.URL.Query().Get("token") != "" {
			token, err := uuid.Parse(r.URL.Query().Get("token"))
			if err != nil {
				tokenID = nil
				tokenSeedName = nil
			} else {
				tokenID = &token
			}
		}

		err := c.service.DeleteAndFactoryResetByName(r.Context(), name, tokenID, tokenSeedName)
		if err != nil {
			return response.SmartError(err)
		}

		return response.EmptySyncResponse
	}

	err := c.service.DeleteByName(r.Context(), name, deleteMode == api.ClusterDeleteModeForce)
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

// swagger:operation POST /1.0/provisioning/clusters/{name}/:resync-inventory clusters cluster_resync_inventory_post
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

// swagger:operation PUT /1.0/provisioning/clusters/{name}/certificate clusters cluster_certificate_put
//
//	Update the cluster's certificate and key
//
//	Update the cluster's certificate and key.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster_certificate_put
//	    description: Cluster certificate definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ClusterCertificatePut"
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: Cluster certificate update response
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
func (c *clusterHandler) clusterCertificatePut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var request api.ClusterCertificatePut

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		return response.BadRequest(err)
	}

	err = c.service.UpdateCertificate(r.Context(), name, request.ClusterCertificate, request.ClusterCertificateKey)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update certificate for cluster: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/provisioning/clusters/{clusterName}/artifacts clusters cluster_artifacts_get
//
//	Get a cluster's artifacts
//
//	Returns a list of a cluster's artifacts (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API cluster artifacts
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
//	          description: List of cluster artifacts
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/provisioning/clusters/one/artifacts/one",
//	              "/1.0/provisioning/clusters/one/artifacts/two"
//	            ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/clusters/{clusterName}/artifacts?recursion=1 clusters cluster_artifacts_get_recursion
//
//	Get the cluster's artifacts
//
//	Returns a list of a cluster's artifacts (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API cluster artifacts
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
//	          description: List of cluster's artifacts
//	          items:
//	            $ref: "#/definitions/ClusterArtifact"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterArtifactsGet(r *http.Request) response.Response {
	clusterName := r.PathValue("clusterName")

	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		clusterArtifacts, err := c.service.GetClusterArtifactAll(r.Context(), clusterName)
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.ClusterArtifact, 0, len(clusterArtifacts))
		for _, artifact := range clusterArtifacts {
			files := make([]api.ClusterArtifactFile, 0, len(artifact.Files))
			for _, file := range artifact.Files {
				files = append(files, api.ClusterArtifactFile{
					Name:     file.Name,
					MimeType: file.MimeType,
					Size:     file.Size,
				})
			}

			result = append(result, api.ClusterArtifact{
				Name:        artifact.Name,
				Cluster:     artifact.Cluster,
				Description: artifact.Description,
				Properties:  artifact.Properties,
				Files:       files,
				LastUpdated: artifact.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	clusterArtifactNames, err := c.service.GetClusterArtifactAllNames(r.Context(), clusterName)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(clusterArtifactNames))
	for _, artifactName := range clusterArtifactNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/clusters/%s/artifacts/%s", api.APIVersion, clusterName, artifactName))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/provisioning/clusters/{clusterName}/artifacts/{artifactName} clusters cluster_artifact_get
//
//	Get a cluster's artifact
//
//	Gets a specific cluster's artifact.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Cluster Artifact
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
//	          $ref: "#/definitions/ClusterArtifact"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/clusters/{clusterName}/artifacts/{artifactName}?archive=zip clusters cluster_artifact_get_archive
//
//	Get a cluster's artifact as archive
//
//	Gets a specific cluster's artifact as archive. The "archive" query parameter
//	takes the archive format, which should be returned. As of now, "zip" is
//	the only value supported.
//
//	---
//	produces:
//	  - application/zip
//	parameters:
//	  - in: query
//	    name: archive
//	    description: |-
//	      Format of the archive to be returned. As of now, "zip" is the only
//	      format supported.
//	    type: string
//	    example: zip
//	responses:
//	  "200":
//	    description: Zip Archive of all the files of the artifact.
//	    "application/zip":
//	      schema:
//	        type: string
//	        format: binary
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterArtifactGet(r *http.Request) response.Response {
	clusterName := r.PathValue("clusterName")
	artifactName := r.PathValue("artifactName")

	// Parse the archive query parameter.
	archive := r.FormValue("archive")
	if archive != "" {
		archiveType, ok := provisioning.ClusterArtifactArchiveTypes[archive]
		if !ok {
			return response.BadRequest(fmt.Errorf("Archive type %q not supported", archive))
		}

		rc, size, err := c.service.GetClusterArtifactArchiveByName(r.Context(), clusterName, artifactName, archiveType)
		if err != nil {
			return response.SmartError(fmt.Errorf("Failed to get archive for artifact %q of cluster %q: %w", artifactName, clusterName, err))
		}

		// Prevent double compression of already compressed archive types.
		if archiveType.Compressed {
			r.Header.Del("Accept-Encoding")
		}

		filename := fmt.Sprintf("%s-%s.%s", clusterName, artifactName, archiveType.Ext)
		headers := map[string]string{
			"Content-Type": archiveType.MimeType,
		}

		return response.ReadCloserResponse(r, rc, false, filename, size, headers)
	}

	artifact, err := c.service.GetClusterArtifactByName(r.Context(), clusterName, artifactName)
	if err != nil {
		return response.SmartError(err)
	}

	files := make([]api.ClusterArtifactFile, 0, len(artifact.Files))
	for _, file := range artifact.Files {
		files = append(files, api.ClusterArtifactFile{
			Name:     file.Name,
			MimeType: file.MimeType,
			Size:     file.Size,
		})
	}

	return response.SyncResponseETag(
		true,
		api.ClusterArtifact{
			Name:        artifact.Name,
			Cluster:     artifact.Cluster,
			Description: artifact.Description,
			Properties:  artifact.Properties,
			Files:       files,
			LastUpdated: artifact.LastUpdated,
		},
		artifact,
	)
}

// swagger:operation GET /1.0/provisioning/clusters/{clusterName}/artifacts/{artifactName}/{filename} clusters cluster_artifact_file_get
//
//	Get a specific file from a cluster's artifact
//
//	Gets a specific file from a cluster's artifact.
//
//	---
//	produces:
//	  - "*/*"
//	responses:
//	  "200":
//	    description: File content.
//	    "*/*":
//	      schema:
//	        type: string
//	        format: binary
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterHandler) clusterArtifactFileGet(r *http.Request) response.Response {
	clusterName := r.PathValue("clusterName")
	artifactName := r.PathValue("artifactName")
	filename := r.PathValue("filename")

	artifact, err := c.service.GetClusterArtifactByName(r.Context(), clusterName, artifactName)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get artifact %q for cluster %q: %w", artifactName, clusterName, err))
	}

	found := false
	var file provisioning.ClusterArtifactFile
	for _, file = range artifact.Files {
		if file.Name == filename {
			found = true
			break
		}
	}

	if !found {
		return response.NotFound(fmt.Errorf("File %q not found in artifact %q", filename, fmt.Sprintf("/%s/provisioning/clusters/%s/artifacts/%s", api.APIVersion, clusterName, artifactName)))
	}

	headers := map[string]string{
		"Content-Type": file.MimeType,
	}

	rc, err := file.Open()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to open %q for reading: %w", filename, err))
	}

	return response.ReadCloserResponse(r, rc, false, filename, int(file.Size), headers)
}
