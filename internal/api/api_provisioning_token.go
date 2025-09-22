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
	service    provisioning.TokenService
	authorizer *authz.Authorizer
}

func registerProvisioningTokenHandler(router Router, authorizer *authz.Authorizer, service provisioning.TokenService) {
	handler := &tokenHandler{
		service:    service,
		authorizer: authorizer,
	}

	// Authentication and authorization are only required, if the respective token image seed is not public.
	router.HandleFunc("GET /{uuid}/seeds/{name}", response.With(handler.tokenSeedGet))

	// Normal authentication and authorization rules apply.
	router.HandleFunc("GET /{$}", response.With(handler.tokensGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.tokensPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{uuid}", response.With(handler.tokenGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{uuid}", response.With(handler.tokenPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{uuid}", response.With(handler.tokenDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{uuid}/image", response.With(handler.tokenImagePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}/seeds", response.With(handler.tokenSeedsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{uuid}/seeds", response.With(handler.tokenSeedsPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("PUT /{uuid}/seeds/{name}", response.With(handler.tokenSeedPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{uuid}/seeds/{name}", response.With(handler.tokenSeedDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
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
//	  - application/json
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

	rc, err := t.service.GetPreSeedImage(r.Context(), UUID, tokenImagePost.Type, provisioning.TokenImageSeedConfigs{
		Applications: tokenImagePost.Seeds.Applications,
		Network:      tokenImagePost.Seeds.Network,
		Install:      tokenImagePost.Seeds.Install,
	})
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, rc, true, fmt.Sprintf("pre-seed-%s%s", UUID.String(), tokenImagePost.Type.FileExt()), -1, nil)
}

// swagger:operation POST /1.0/provisioning/tokens/{uuid}/seeds tokens token_seeds_post
//
//	Add a token seed configuration
//
//	Add a token seed configuration for later IncusOS ISO or raw image generation.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: tokenSeedsPost
//	    description: Token seed configuration record.
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenSeedsPost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenSeedsPost(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	var tokenSeedsPost api.TokenSeedPost
	err = json.NewDecoder(r.Body).Decode(&tokenSeedsPost)
	if err != nil {
		return response.BadRequest(err)
	}

	seedConfig, err := t.service.CreateTokenSeed(r.Context(), provisioning.TokenSeed{
		Token:       UUID,
		Name:        tokenSeedsPost.Name,
		Description: tokenSeedsPost.Description,
		Public:      tokenSeedsPost.Public,
		Seeds: provisioning.TokenImageSeedConfigs{
			Applications: tokenSeedsPost.Seeds.Applications,
			Network:      tokenSeedsPost.Seeds.Network,
			Install:      tokenSeedsPost.Seeds.Install,
		},
	})
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/tokens/"+UUID.String()+"/images/"+seedConfig.Name)
}

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/seeds tokens tokens_seeds_get
//
//	Get the token seed configs
//
//	Returns a list of seed configs of a given token (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API token seed configs
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
//	          description: List of token seed configs
//                items:
//                  type: string
//                example: |-
//                  [
//                    "/1.0/provisioning/tokens/b32d0079-c48b-4957-b1cb-bef54125c861/images/first",
//                    "/1.0/provisioning/tokens/b32d0079-c48b-4957-b1cb-bef54125c861/images/second"
//                  ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/seeds?recursion=1 tokens tokens_seeds_get_recursion
//
//	Get the tokens
//
//	Returns a list of token seed configs (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API token seed configs
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
//	          description: List of token seed configs
//	          items:
//	            $ref: "#/definitions/TokenSeed"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenSeedsGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		tokenSeeds, err := t.service.GetTokenSeedAll(r.Context(), UUID)
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.TokenSeed, 0, len(tokenSeeds))
		for _, tokenSeed := range tokenSeeds {
			result = append(result, api.TokenSeed{
				Token:       tokenSeed.Token,
				LastUpdated: tokenSeed.LastUpdated,
				TokenSeedPost: api.TokenSeedPost{
					Name: tokenSeed.Name,
					TokenSeedPut: api.TokenSeedPut{
						Description: tokenSeed.Description,
						Public:      tokenSeed.Public,
						Seeds: api.TokenSeedConfigs{
							Applications: tokenSeed.Seeds.Applications,
							Network:      tokenSeed.Seeds.Network,
							Install:      tokenSeed.Seeds.Install,
						},
					},
				},
			})
		}

		return response.SyncResponse(true, result)
	}

	tokenSeedNames, err := t.service.GetTokenSeedAllNames(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(tokenSeedNames))
	for _, name := range tokenSeedNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/tokens/%s/images/%s", api.APIVersion, UUID, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/seeds/{name} tokens token_seed_get
//
//	Get token seed config
//
//	Get token seed config. This can be the literal config as JSON or it can be
//	the generated pre-seeded IncusOS ISO or raw image file, if the `type`
//	query parameter is provided.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	  - application/octet-stream
//	  - application/gzip
//	parameters:
//	  - in: query
//	    name: type
//	    description: |-
//	      Type of the generated file, "iso" or "raw".
//	      If omitted, the token seed configuration is returned as JSON.
//	responses:
//	  "200":
//	    description: Token seed config
//	    content:
//	      application/json:
//	        description: Token seed config as JSON
//	        schema:
//	          type: object
//	          description: Sync response
//	          properties:
//	            type:
//	              type: string
//	              description: Response type
//	              example: sync
//	            status:
//	              type: string
//	              description: Status description
//	              example: Success
//	            status_code:
//	              type: integer
//	              description: Status code
//	              example: 200
//	            metadata:
//	              $ref: "#/definitions/TokenSeed"
//	      application/octet-stream:
//	        description: Raw file data
//	      application/gzip:
//	        description: Raw file data
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenSeedGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	name := r.PathValue("name")

	typeArg := r.URL.Query().Get("type")

	seedConfig, err := t.service.GetTokenSeedByName(r.Context(), UUID, name)
	if err != nil {
		return response.SmartError(err)
	}

	if !seedConfig.Public {
		// If the requested token seed config is not public, perform regular
		// authorization logic.
		resp := checkPermission(t.authorizer, r, authz.ObjectTypeServer, authz.EntitlementCanView)
		if resp != nil {
			return resp
		}
	}

	if typeArg == "" {
		return response.SyncResponseETag(
			true,
			api.TokenSeed{
				Token: seedConfig.Token,
				TokenSeedPost: api.TokenSeedPost{
					Name: seedConfig.Name,
					TokenSeedPut: api.TokenSeedPut{
						Description: seedConfig.Description,
						Public:      seedConfig.Public,
						Seeds: api.TokenSeedConfigs{
							Applications: seedConfig.Seeds.Applications,
							Network:      seedConfig.Seeds.Network,
							Install:      seedConfig.Seeds.Install,
						},
					},
				},
				LastUpdated: seedConfig.LastUpdated,
			},
			seedConfig,
		)
	}

	imageType := api.ImageType(typeArg)
	if !imageType.IsValid() {
		return response.BadRequest(fmt.Errorf("type %q is not valid", typeArg))
	}

	rc, err := t.service.GetTokenImageFromTokenSeed(r.Context(), UUID, name, imageType)
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, rc, true, fmt.Sprintf("pre-seed-%s%s", name, imageType.FileExt()), -1, nil)
}

// swagger:operation PUT /1.0/provisioning/tokens/{uuid}/seeds/{name} tokens token_seed_put
//
//	Update the token seed config
//
//	Updates the token seed config definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: token
//	    description: Token seed config definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenSeed"
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
func (t *tokenHandler) tokenSeedPut(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	name := r.PathValue("name")

	var tokenSeed api.TokenSeedPut

	err = json.NewDecoder(r.Body).Decode(&tokenSeed)
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

	currentTokenSeed, err := t.service.GetTokenSeedByName(ctx, UUID, name)
	if err != nil {
		return response.SmartError(fmt.Errorf(`Failed to get token seed config "%s/%s": %w`, UUID.String(), name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentTokenSeed)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = t.service.UpdateTokenSeed(ctx, provisioning.TokenSeed{
		ID:          currentTokenSeed.ID,
		Token:       currentTokenSeed.Token,
		Name:        currentTokenSeed.Name,
		Description: tokenSeed.Description,
		Public:      tokenSeed.Public,
		Seeds: provisioning.TokenImageSeedConfigs{
			Applications: tokenSeed.Seeds.Applications,
			Network:      tokenSeed.Seeds.Network,
			Install:      tokenSeed.Seeds.Install,
		},
	})
	if err != nil {
		return response.SmartError(fmt.Errorf(`Failed updating token seed config "%s/%s": %w`, UUID.String(), name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/tokens/"+UUID.String()+"/images/"+name)
}

// swagger:operation DELETE /1.0/provisioning/tokens/{uuid}/seeds/{name} tokens token_seed_delete
//
//	Delete the token seed config
//
//	Removes the token seed config.
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
func (t *tokenHandler) tokenSeedDelete(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	name := r.PathValue("name")

	err = t.service.DeleteTokenSeedByName(r.Context(), UUID, name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
