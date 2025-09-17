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
	"github.com/FuturFusion/operations-center/shared/api"
)

type tokenHandler struct {
	service provisioning.TokenService
}

func registerProvisioningTokenHandler(router Router, authorizer *authz.Authorizer, service provisioning.TokenService) {
	handler := &tokenHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.tokensGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.tokensPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{uuid}", response.With(handler.tokenGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{uuid}", response.With(handler.tokenPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{uuid}", response.With(handler.tokenDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{uuid}/image", response.With(handler.tokenImagePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
}

// swagger:operation GET /1.0/provisioning/tokens tokens tokens_get
//
//	Get the tokens
//
//	Returns a list of tokens (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API tokens
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
//	          description: List of tokens
//                items:
//                  type: string
//                example: |-
//                  [
//                    "/1.0/provisioning/tokens/b32d0079-c48b-4957-b1cb-bef54125c861",
//                    "/1.0/provisioning/tokens/464d229b-3069-4a82-bc59-b215a7c6ed1b"
//                  ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/tokens?recursion=1 tokens tokens_get_recursion
//
//	Get the tokens
//
//	Returns a list of tokens (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API tokens
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
//	          description: List of tokens
//	          items:
//	            $ref: "#/definitions/Token"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokensGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		tokens, err := t.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Token, 0, len(tokens))
		for _, token := range tokens {
			result = append(result, api.Token{
				UUID: token.UUID,
				TokenPut: api.TokenPut{
					UsesRemaining: token.UsesRemaining,
					ExpireAt:      token.ExpireAt,
					Description:   token.Description,
				},
			})
		}

		return response.SyncResponse(true, result)
	}

	tokenIDs, err := t.service.GetAllUUIDs(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(tokenIDs))
	for _, id := range tokenIDs {
		result = append(result, fmt.Sprintf("/%s/provisioning/tokens/%s", api.APIVersion, id.String()))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/tokens tokens tokens_post
//
//	Add a token
//
//	Creates a new token.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: token
//	    description: Token configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenPost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokensPost(r *http.Request) response.Response {
	var token api.TokenPut

	// Decode into the new token.
	err := json.NewDecoder(r.Body).Decode(&token)
	if err != nil {
		return response.BadRequest(err)
	}

	newToken, err := t.service.Create(r.Context(), provisioning.Token{
		UsesRemaining: token.UsesRemaining,
		ExpireAt:      token.ExpireAt,
		Description:   token.Description,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating token: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/tokens/"+newToken.UUID.String())
}

// swagger:operation GET /1.0/provisioning/tokens/{uuid} tokens token_get
//
//	Get the token
//
//	Gets a specific token.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Token
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
//	          $ref: "#/definitions/Token"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	token, err := t.service.GetByUUID(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Token{
			UUID: token.UUID,
			TokenPut: api.TokenPut{
				UsesRemaining: token.UsesRemaining,
				ExpireAt:      token.ExpireAt,
				Description:   token.Description,
			},
		},
		token,
	)
}

// swagger:operation PUT /1.0/provisioning/tokens/{uuid} tokens token_put
//
//	Update the token
//
//	Updates the token definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: token
//	    description: Token definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/Token"
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
func (t *tokenHandler) tokenPut(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	var token api.Token

	err = json.NewDecoder(r.Body).Decode(&token)
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

	currentToken, err := t.service.GetByUUID(ctx, UUID)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get token %q: %w", UUID.String(), err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentToken)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = t.service.Update(ctx, provisioning.Token{
		UUID:          currentToken.UUID,
		UsesRemaining: token.UsesRemaining,
		ExpireAt:      token.ExpireAt,
		Description:   token.Description,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating token %q: %w", UUID.String(), err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/tokens/"+UUID.String())
}

// swagger:operation DELETE /1.0/provisioning/tokens/{uuid} tokens token_delete
//
//	Delete the token
//
//	Removes the token.
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
func (t *tokenHandler) tokenDelete(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	err = t.service.DeleteByUUID(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/tokens/{uuid}/image tokens token_image_post
//
//	Generate pre-seed IncusOS ISO or raw image
//
//	Generate and retrieve pre-seed IncusOS ISO or raw image file.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/octet-stream
//	  - application/gzip
//	parameters:
//	  - in: body
//	    name: tokenImagePost
//	    description: Seed configuration for the generated ISO or raw image.
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenImagePost"
//	responses:
//	  "200":
//	    description: Raw file data
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenImagePost(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	var tokenImagePost api.TokenImagePost
	err = json.NewDecoder(r.Body).Decode(&tokenImagePost)
	if err != nil {
		return response.BadRequest(err)
	}

	rc, err := t.service.GetPreSeedImage(r.Context(), UUID, tokenImagePost.Type, provisioning.TokenSeeds{
		Applications: tokenImagePost.Seeds.Applications,
		Network:      tokenImagePost.Seeds.Network,
		Install:      tokenImagePost.Seeds.Install,
	})
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, rc, true, fmt.Sprintf("pre-seed-%s%s", UUID.String(), tokenImagePost.Type.FileExt()), -1, nil)
}
