package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type clusterConfigTemplateHandler struct {
	service provisioning.ClusterTemplateService
}

func registerProvisioningClusterTemplateHandler(router Router, authorizer *authz.Authorizer, service provisioning.ClusterTemplateService) {
	handler := &clusterConfigTemplateHandler{
		service: service,
	}

	// ClusterConfigTemplate config templates
	router.HandleFunc("GET /{$}", response.With(handler.clusterTemplatesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.clusterTemplatesPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{name}", response.With(handler.clusterTemplateGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.clusterTemplatePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.clusterTemplateDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}", response.With(handler.clusterTemplatePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/provisioning/cluster-templates cluster-templates cluster_config_templates_get
//
//	Get the cluster config templates
//
//	Returns a list of cluster config templates (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API cluster config templates
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
//	          description: List of cluster config templates
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/provisioning/cluster-templates/one",
//	              "/1.0/provisioning/cluster-templates/two"
//	            ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/cluster-templates?recursion=1 cluster-templates cluster_config_templates_get_recursion
//
//	Get the cluster config templates
//
//	Returns a list of cluster config templates (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API cluster config templates
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
//	          description: List of cluster config templates
//	          items:
//	            $ref: "#/definitions/ClusterConfigTemplate"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterConfigTemplateHandler) clusterTemplatesGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		clusterConfigTemplates, err := c.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.ClusterTemplate, 0, len(clusterConfigTemplates))
		for _, clusterConfigTemplate := range clusterConfigTemplates {
			result = append(result, api.ClusterTemplate{
				ClusterTemplatePost: api.ClusterTemplatePost{
					Name: clusterConfigTemplate.Name,
					ClusterTemplatePut: api.ClusterTemplatePut{
						Description:               clusterConfigTemplate.Description,
						ServiceConfigTemplate:     clusterConfigTemplate.ServiceConfigTemplate,
						ApplicationConfigTemplate: clusterConfigTemplate.ApplicationConfigTemplate,
						Variables:                 clusterConfigTemplate.Variables,
					},
				},
				LastUpdated: clusterConfigTemplate.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	clusterConfigTemplateNames, err := c.service.GetAllNames(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(clusterConfigTemplateNames))
	for _, name := range clusterConfigTemplateNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/cluster-templates/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/cluster-templates cluster-templates cluster_config_templates_post
//
//	Add a cluster config template
//
//	Creates a new cluster config template.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster-config-template
//	    description: ClusterConfigTemplate configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ClusterTemplatePost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterConfigTemplateHandler) clusterTemplatesPost(r *http.Request) response.Response {
	var clusterConfigTemplate api.ClusterTemplatePost

	// Decode into the new clusterConfigTemplate.
	err := json.NewDecoder(r.Body).Decode(&clusterConfigTemplate)
	if err != nil {
		return response.BadRequest(err)
	}

	_, err = c.service.Create(r.Context(), provisioning.ClusterTemplate{
		Name:                      clusterConfigTemplate.Name,
		Description:               clusterConfigTemplate.Description,
		ServiceConfigTemplate:     clusterConfigTemplate.ServiceConfigTemplate,
		ApplicationConfigTemplate: clusterConfigTemplate.ApplicationConfigTemplate,
		Variables:                 clusterConfigTemplate.Variables,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating cluster config template: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/cluster-templates/"+clusterConfigTemplate.Name)
}

// swagger:operation GET /1.0/provisioning/cluster-templates/{name} cluster-templates cluster_config_template_get
//
//	Get the cluster config template
//
//	Gets a specific cluster config template.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: ClusterConfigTemplate
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
//	          $ref: "#/definitions/ClusterConfigTemplate"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (c *clusterConfigTemplateHandler) clusterTemplateGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	clusterConfigTemplate, err := c.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.ClusterTemplate{
			ClusterTemplatePost: api.ClusterTemplatePost{
				Name: clusterConfigTemplate.Name,
				ClusterTemplatePut: api.ClusterTemplatePut{
					Description:               clusterConfigTemplate.Description,
					ServiceConfigTemplate:     clusterConfigTemplate.ServiceConfigTemplate,
					ApplicationConfigTemplate: clusterConfigTemplate.ApplicationConfigTemplate,
					Variables:                 clusterConfigTemplate.Variables,
				},
			},
			LastUpdated: clusterConfigTemplate.LastUpdated,
		},
		clusterConfigTemplate,
	)
}

// swagger:operation PUT /1.0/provisioning/cluster-templates/{name} cluster-templates cluster_config_template_put
//
//	Update the cluster config template
//
//	Updates the cluster config template definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster_config_template
//	    description: ClusterConfigTemplate definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ClusterConfigTemplate"
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
func (c *clusterConfigTemplateHandler) clusterTemplatePut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var clusterConfigTemplate api.ClusterTemplate

	err := json.NewDecoder(r.Body).Decode(&clusterConfigTemplate)
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

	currentClusterConfigTemplate, err := c.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get clusterConfigTemplate %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentClusterConfigTemplate)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	currentClusterConfigTemplate.Description = clusterConfigTemplate.Description
	currentClusterConfigTemplate.ServiceConfigTemplate = clusterConfigTemplate.ServiceConfigTemplate
	currentClusterConfigTemplate.ApplicationConfigTemplate = clusterConfigTemplate.ApplicationConfigTemplate
	currentClusterConfigTemplate.Variables = clusterConfigTemplate.Variables

	err = c.service.Update(ctx, *currentClusterConfigTemplate)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating cluster config template %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/cluster-templates/"+name)
}

// swagger:operation DELETE /1.0/provisioning/cluster-templates/{name} cluster-templates cluster_config_template_delete
//
//	Delete the cluster config template
//
//	Removes the cluster config template.
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
func (c *clusterConfigTemplateHandler) clusterTemplateDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := c.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/cluster-templates/{name} cluster-templates cluster_config_template_post
//
//	Rename the cluster config template
//
//	Renames the cluster config template.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: cluster_config_template
//	    description: ClusterConfigTemplate definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ClusterConfigTemplate"
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
func (c *clusterConfigTemplateHandler) clusterTemplatePost(r *http.Request) response.Response {
	name := r.PathValue("name")

	var clusterConfigTemplate api.ClusterTemplate

	err := json.NewDecoder(r.Body).Decode(&clusterConfigTemplate)
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

	currentClusterConfigTemplate, err := c.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get cluster config template %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentClusterConfigTemplate)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = c.service.Rename(ctx, name, clusterConfigTemplate.Name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed renaming cluster config template %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/cluster-templates/"+clusterConfigTemplate.Name)
}
