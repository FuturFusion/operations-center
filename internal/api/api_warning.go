package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type warningHandler struct {
	service    warning.WarningService
	authorizer *authz.Authorizer
}

func registerWarningHandler(router Router, authorizer *authz.Authorizer, service warning.WarningService) {
	handler := &warningHandler{
		service:    service,
		authorizer: authorizer,
	}

	// Normal authentication and authorization rules apply.
	router.HandleFunc("GET /{$}", response.With(handler.warningsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}", response.With(handler.warningGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{uuid}", response.With(handler.warningPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/warnings warnings warnings_get_recursion
//
//	Get the warnings
//
//	Returns a list of warnings (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API warnings
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
//	          description: List of warnings
//	          items:
//	            $ref: "#/definitions/Warning"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *warningHandler) warningsGet(r *http.Request) response.Response {
	warnings, err := t.service.GetAll(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]api.Warning, 0, len(warnings))
	for _, warn := range warnings {
		result = append(result, api.Warning{
			UUID: warn.UUID,
			Scope: api.WarningScope{
				Scope:      warn.Scope,
				EntityType: warn.EntityType,
				Entity:     warn.Entity,
			},
			Type:            warn.Type,
			FirstOccurrence: warn.FirstOccurrence,
			LastOccurrence:  warn.LastOccurrence,
			LastUpdated:     warn.LastUpdated,
			Messages:        warn.Messages,
			Count:           warn.Count,
			WarningPut: api.WarningPut{
				Status: warn.Status,
			},
		})
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/warnings/{uuid} warnings warning_get
//
//	Get the warning
//
//	Gets a specific warning.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Warning
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
//	          $ref: "#/definitions/Warning"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *warningHandler) warningGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	warn, err := t.service.GetByUUID(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Warning{
			UUID: warn.UUID,
			Scope: api.WarningScope{
				Scope:      warn.Scope,
				EntityType: warn.EntityType,
				Entity:     warn.Entity,
			},
			Type:            warn.Type,
			FirstOccurrence: warn.FirstOccurrence,
			LastOccurrence:  warn.LastOccurrence,
			LastUpdated:     warn.LastUpdated,
			Messages:        warn.Messages,
			Count:           warn.Count,
			WarningPut: api.WarningPut{
				Status: warn.Status,
			},
		},
		warn,
	)
}

// swagger:operation PUT /1.0/warnings/{uuid} warnings warning_put
//
//	Update the warning
//
//	Updates the warning definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: warning
//	    description: Warning definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/WarningPut"
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
func (t *warningHandler) warningPut(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	var warningPut api.WarningPut

	err = json.NewDecoder(r.Body).Decode(&warningPut)
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

	currentWarning, err := t.service.GetByUUID(ctx, UUID)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get warning %q: %w", UUID.String(), err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentWarning)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	_, err = t.service.UpdateStatusByUUID(ctx, UUID, warningPut.Status)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating warning %q: %w", UUID.String(), err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/warnings/"+UUID.String())
}
