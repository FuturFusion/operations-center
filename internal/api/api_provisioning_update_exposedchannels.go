package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type updateExposedchannelsHandler struct {
	service provisioning.UpdateService
}

func registerUpdateExposedchannelsHandler(router Router, authorizer *authz.Authorizer, service provisioning.UpdateService) {
	handler := &updateExposedchannelsHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.updateExposedchannelsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.updateExposedchannelsPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{name}", response.With(handler.updateExposedchannelGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.updateExposedchannelPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.updateExposedchannelDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}", response.With(handler.updateExposedchannelPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/provisioning/update-exposedchannels exposedchannels exposedchannels_get
//
//	Get the exposed channels
//
//	Returns a list of exposed channels for updates (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API exposed channels
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
//	          description: List of exposed channels
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/provisioning/update-exposedchannels/one",
//	              "/1.0/provisioning/update-exposedchannels/two"
//	            ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/update-exposedchannels?recursion=1 exposedchannels exposedchannels_get_recursion
//
//	Get the exposed channels
//
//	Returns a list of exposed channels for updates (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API exposed channels
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
//	          description: List of exposedchannels
//	          items:
//	            $ref: "#/definitions/UpdateExposedchannel"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateExposedchannelsHandler) updateExposedchannelsGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		exposedchannels, err := u.service.GetAllExposedchannels(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.UpdateExposedchannel, 0, len(exposedchannels))
		for _, exposedchannel := range exposedchannels {
			result = append(result, api.UpdateExposedchannel{
				UpdateExposedchannelPost: api.UpdateExposedchannelPost{
					Name: exposedchannel.Name,
					UpdateExposedchannelPut: api.UpdateExposedchannelPut{
						Description: exposedchannel.Description,
					},
				},
				LastUpdated: exposedchannel.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	exposedchannelNames, err := u.service.GetAllExposedchannelNames(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(exposedchannelNames))
	for _, name := range exposedchannelNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/update-exposedchannels/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/update-exposedchannels exposedchannels exposedchannels_post
//
//	Add an exposed channel
//
//	Creates a new exposed channel for updates.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: exposedchannel
//	    description: Exposed channel configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/UpdateExposedchannelPost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateExposedchannelsHandler) updateExposedchannelsPost(r *http.Request) response.Response {
	var exposedchannel api.UpdateExposedchannelPost

	// Decode into the new exposedchannel.
	err := json.NewDecoder(r.Body).Decode(&exposedchannel)
	if err != nil {
		return response.BadRequest(err)
	}

	_, err = u.service.CreateExposedchannel(r.Context(), provisioning.Exposedchannel{
		Name:        exposedchannel.Name,
		Description: exposedchannel.Description,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating exposed channel: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/update-exposedchannels/"+exposedchannel.Name)
}

// swagger:operation GET /1.0/provisioning/update-exposedchannels/{name} exposedchannels exposedchannel_get
//
//	Get the exposed channel
//
//	Gets a specific exposed channel for updates.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Exposed channel
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
//	          $ref: "#/definitions/UpdateExposedchannel"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateExposedchannelsHandler) updateExposedchannelGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	exposedchannel, err := u.service.GetExposedchannelByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.UpdateExposedchannel{
			UpdateExposedchannelPost: api.UpdateExposedchannelPost{
				Name: exposedchannel.Name,
				UpdateExposedchannelPut: api.UpdateExposedchannelPut{
					Description: exposedchannel.Description,
				},
			},
			LastUpdated: exposedchannel.LastUpdated,
		},
		exposedchannel,
	)
}

// swagger:operation PUT /1.0/provisioning/update-exposedchannels/{name} exposedchannels exposedchannel_put
//
//	Update the exposed channel
//
//	Updates the exposed channel definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: exposed channel
//	    description: Exposed channel definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/UpdateExposedchannelPut"
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
func (u *updateExposedchannelsHandler) updateExposedchannelPut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var exposedchannel api.UpdateExposedchannelPut

	err := json.NewDecoder(r.Body).Decode(&exposedchannel)
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

	currentExposedchannel, err := u.service.GetExposedchannelByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get exposed channel %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentExposedchannel)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	currentExposedchannel.Description = exposedchannel.Description

	err = u.service.UpdateExposedchannel(ctx, *currentExposedchannel)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating exposed channel %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/update-exposedchannels/"+name)
}

// swagger:operation DELETE /1.0/provisioning/update-exposedchannels/{name} exposedchannels exposedchannel_delete
//
//	Delete the exposed channel
//
//	Removes the exposed channel.
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
func (u *updateExposedchannelsHandler) updateExposedchannelDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := u.service.DeleteExposedchannelByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/update-exposedchannels/{name} exposedchannels exposedchannel_post
//
//	Rename the exposed channel
//
//	Renames the exposed channel.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: exposed channel
//	    description: Exposed channel definition.
//	    required: true
//	    schema:
//	      $ref: "#/definitions/UpdateExposedchannelPost"
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
func (u *updateExposedchannelsHandler) updateExposedchannelPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	var exposedchannel api.UpdateExposedchannelPost

	err := json.NewDecoder(r.Body).Decode(&exposedchannel)
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

	currentExposedchannel, err := u.service.GetExposedchannelByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get exposed channel %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentExposedchannel)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = u.service.RenameExposedchannel(ctx, name, exposedchannel.Name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed renaming exposed channel %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/update-exposedchannel/"+exposedchannel.Name)
}
