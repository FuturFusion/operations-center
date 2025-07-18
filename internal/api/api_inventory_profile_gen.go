// Code generated by generate-inventory; DO NOT EDIT.

package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type profileHandler struct {
	service inventory.ProfileService
}

func registerInventoryProfileHandler(router Router, authorizer authz.Authorizer, service inventory.ProfileService) {
	handler := &profileHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.profilesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}", response.With(handler.profileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{uuid}/resync", response.With(handler.profileResyncPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/inventory/profiles profiles profiles_get
//
//	Get the profiles
//
//	Returns a list of profiles (list of relative URLs).
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
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API profiles
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
//	          description: List of profiles
//	               items:
//	                 type: string
//	               example: |-
//	                 [
//	                   "/1.0/inventory/profiles/1",
//	                   "/1.0/inventory/profiles/2"
//	                 ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/inventory/profiles?recursion=1 profiles profiles_get_recursion
//
//	Get the profiles
//
//	Returns a list of profiles (structs).
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
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API profiles
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
//	          description: List of profiles
//	          items:
//	            $ref: "#/definitions/profile"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *profileHandler) profilesGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	var filter inventory.ProfileFilter

	if r.URL.Query().Get("cluster") != "" {
		filter.Cluster = ptr.To(r.URL.Query().Get("cluster"))
	}

	if r.URL.Query().Get("project") != "" {
		filter.Project = ptr.To(r.URL.Query().Get("project"))
	}

	if r.URL.Query().Get("filter") != "" {
		filter.Expression = ptr.To(r.URL.Query().Get("filter"))
	}

	if recursion == 1 {
		// FIXME: Should we require a non empty filter with recursion?
		profiles, err := i.service.GetAllWithFilter(r.Context(), filter)
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Profile, 0, len(profiles))
		for _, profile := range profiles {
			result = append(result, api.Profile{
				UUID:        profile.UUID,
				Cluster:     profile.Cluster,
				ProjectName: profile.ProjectName,
				Name:        profile.Name,
				Object:      profile.Object,
				LastUpdated: profile.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	profileUUIDs, err := i.service.GetAllUUIDsWithFilter(r.Context(), filter)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(profileUUIDs))
	for _, id := range profileUUIDs {
		result = append(result, fmt.Sprintf("/%s/inventory/profile/%d", api.APIVersion, id))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/inventory/profiles/{uuid} profiles profile_get
//
//	Get the profile
//
//	Gets a specific profile.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: profile
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
//	          $ref: "#/definitions/Profile"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *profileHandler) profileGet(r *http.Request) response.Response {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		return response.SmartError(err)
	}

	profile, err := i.service.GetByUUID(r.Context(), id)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(
		true,
		api.Profile{
			UUID:        profile.UUID,
			Cluster:     profile.Cluster,
			ProjectName: profile.ProjectName,
			Name:        profile.Name,
			Object:      profile.Object,
			LastUpdated: profile.LastUpdated,
		},
	)
}

// swagger:operation POST /1.0/inventory/profiles/{uuid}/resync profiles profile_get_resync_post
//
//	Resync the profile
//
//	Resync a specific profile.
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
func (i *profileHandler) profileResyncPost(r *http.Request) response.Response {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		return response.SmartError(err)
	}

	err = i.service.ResyncByUUID(r.Context(), id)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to resync profile: %w", err))
	}

	return response.EmptySyncResponse
}
