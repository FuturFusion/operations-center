package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/decodestrict"
	"github.com/FuturFusion/operations-center/internal/util/response"
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

	// Authentication and authorization are only required, if the respective token seed is not defined as public.
	router.HandleFunc("GET /{uuid}/seeds/{name}/{params...}", response.With(handler.tokenSeedImageGet))

	// Normal authentication and authorization rules apply.
	router.HandleFunc("GET /{$}", response.With(handler.tokensGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.tokensPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{uuid}", response.With(handler.tokenGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{uuid}", response.With(handler.tokenPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{uuid}", response.With(handler.tokenDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{uuid}/image", response.With(handler.tokenImagePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}/image/{imageUUID}", response.With(handler.tokenImageGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}/provider-config", response.With(handler.tokenProviderConfigGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}/seeds", response.With(handler.tokenSeedsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{uuid}/seeds/{name}", response.With(handler.tokenSeedGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
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
//	    $ref: "#/responses/URLsResponse"
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
//	    $ref: "#/responses/TokensResponse"
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
					Channel:       token.Channel,
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
//	      $ref: "#/definitions/TokenPut"
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
		return response.BadRequest(fmt.Errorf("Failed to decode token: %w", err))
	}

	newToken, err := t.service.Create(r.Context(), provisioning.Token{
		UsesRemaining: token.UsesRemaining,
		ExpireAt:      token.ExpireAt,
		Description:   token.Description,
		Channel:       token.Channel,
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
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/TokenResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
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
				Channel:       token.Channel,
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
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
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
//	  "404":
//	    $ref: "#/responses/NotFound"
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
		return response.BadRequest(fmt.Errorf("Failed to decode token: %w", err))
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
		Channel:       token.Channel,
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
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
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
//	Prepare pre-seed IncusOS ISO or raw image download
//
//	Prepare pre-seed IncusOS ISO or raw image download.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: body
//	    name: tokenImagePost
//	    description: Seed configuration for the generated ISO or raw image.
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenImagePost"
//	responses:
//	  "200":
//	    $ref: "#/responses/TokenImageResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenImagePost(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	var tokenImagePost api.TokenImagePost
	err = decodestrict.JSON(r.Body, &tokenImagePost)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Failed to decode image request: %w", err))
	}

	imageUUID, err := t.service.PreparePreSeededImage(r.Context(), UUID, tokenImagePost.Type, tokenImagePost.Architecture, provisioning.TokenImageSeedConfigs{
		Applications:     tokenImagePost.Seeds.Applications,
		Install:          tokenImagePost.Seeds.Install,
		MigrationManager: tokenImagePost.Seeds.MigrationManager,
		Network:          tokenImagePost.Seeds.Network,
		OperationsCenter: tokenImagePost.Seeds.OperationsCenter,
	})
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseLocation(true, map[string]any{"image": "/" + api.APIVersion + "/provisioning/tokens/" + UUID.String() + "/image/" + imageUUID.String()}, "/"+api.APIVersion+"/provisioning/tokens/image/"+imageUUID.String())
}

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/image/{imageUUID} tokens token_image_get
//
//	Get pre-seed IncusOS ISO or raw image
//
//	Retrieve pre-seed IncusOS ISO or raw image file.
//
//	---
//	produces:
//	  - application/json
//	  - application/octet-stream
//	  - application/gzip
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: path
//	    name: imageUUID
//	    description: UUID of the image
//	    type: string
//	    format: uuid
//	    required: true
//	responses:
//	  "200":
//	    description: Raw file data
//	    schema:
//	      type: file
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenImageGet(r *http.Request) response.Response {
	tokenUUIDString := r.PathValue("uuid")

	tokenUUID, err := uuid.Parse(tokenUUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	imageUUIDString := r.PathValue("imageUUID")

	imageUUID, err := uuid.Parse(imageUUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	rc, filename, err := t.service.GetPreSeededImage(r.Context(), tokenUUID, imageUUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, rc, true, filename, -1, nil)
}

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/provider-config tokens tokens_provider_config_get
//
//	Get the provider config for a token
//
//	Gets the token specific provider config.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/TokenProviderConfigResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenProviderConfigGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	providerConfig, err := t.service.GetTokenProviderConfig(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		providerConfig,
		providerConfig,
	)
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
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: body
//	    name: tokenSeedsPost
//	    description: Token seed configuration record.
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenSeedPost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenSeedsPost(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	var tokenSeedsPost api.TokenSeedPost
	err = decodestrict.JSON(r.Body, &tokenSeedsPost)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Failed to decode token seed: %w", err))
	}

	seedConfig, err := t.service.CreateTokenSeed(r.Context(), provisioning.TokenSeed{
		Token:       UUID,
		Name:        tokenSeedsPost.Name,
		Description: tokenSeedsPost.Description,
		Public:      tokenSeedsPost.Public,
		Seeds: provisioning.TokenImageSeedConfigs{
			Applications:     tokenSeedsPost.Seeds.Applications,
			Install:          tokenSeedsPost.Seeds.Install,
			MigrationManager: tokenSeedsPost.Seeds.MigrationManager,
			Network:          tokenSeedsPost.Seeds.Network,
			OperationsCenter: tokenSeedsPost.Seeds.OperationsCenter,
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
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/URLsResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/seeds?recursion=1 tokens tokens_seeds_get_recursion
//
//	Get the token seed configs
//
//	Returns a list of seed configs of a given token (structs).
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/TokenSeedsResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
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
							Applications:     tokenSeed.Seeds.Applications,
							Install:          tokenSeed.Seeds.Install,
							MigrationManager: tokenSeed.Seeds.MigrationManager,
							Network:          tokenSeed.Seeds.Network,
							OperationsCenter: tokenSeed.Seeds.OperationsCenter,
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
//	Get the token seed config as JSON.
//
//	The generated pre-seeded IncusOS ISO or raw image file is served by
//	`GET /1.0/provisioning/tokens/{uuid}/seeds/{name}/{params}` instead.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the seed
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/TokenSeedResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenSeedGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	name := r.PathValue("name")

	seedConfig, err := t.service.GetTokenSeedByName(r.Context(), UUID, name)
	if err != nil {
		return response.SmartError(err)
	}

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
						Applications:     seedConfig.Seeds.Applications,
						Install:          seedConfig.Seeds.Install,
						MigrationManager: seedConfig.Seeds.MigrationManager,
						Network:          seedConfig.Seeds.Network,
						OperationsCenter: seedConfig.Seeds.OperationsCenter,
					},
				},
			},
			LastUpdated: seedConfig.LastUpdated,
		},
		seedConfig,
	)
}

// swagger:operation GET /1.0/provisioning/tokens/{uuid}/seeds/{name}/{params} tokens token_seed_image_get
//
//	Get pre-seeded IncusOS image
//
//	Get the generated pre-seeded IncusOS ISO or raw image file for a token
//	seed, with all parameters and the terminal filename encoded as path
//	segments and with the filename having a recognized file extension
//	(e.g. ".iso" or ".raw"). This is required by some BMC/Redfish
//	implementations that reject virtual media image URLs which don't end in a
//	known media extension.
//
//	The path after the seed name is a flat sequence of "<key>/<value>" pairs
//	followed by a final filename segment, e.g.:
//
//	  /1.0/provisioning/tokens/{uuid}/seeds/{name}/architecture/x86_64/channel/stable/type/iso/file.iso
//
//	Recognized keys are "architecture" (required), "type" (required) and
//	"channel" (optional, defaults to the configured default update channel).
//
//	The filename segment picks one of two ways of asking for an image.
//
//	A filename of twelve characters out of "A-Za-z0-9_-" names one already
//	generated image, e.g. "a1B2c3D4e5F6.iso". Such an image is served exactly
//	as it was generated. It is the form Operations Center puts into the virtual
//	media URL it hands to a BMC. An image, which has not been generated, is
//	reported as "404 Not Found" rather than being generated on demand.
//	Such an image is only reachable while it is cached, and only exists if the
//	token seed was public when it was generated.
//
//	Any other filename, "file.iso" among them, asks for whatever image the
//	token seed resolves to right now. That image is looked up, generated if
//	need be, and served subject to the usual authorization for a token seed,
//	which is not public.
//
//	The image is served uncompressed, with a Content-Length header and support
//	for byte ranges, so a BMC whose transfer got interrupted can resume it
//	instead of fetching the whole image again.
//	A client asking for "Accept: application/gzip" and not requesting a range
//	gets the gzip compressed image as a ".gz" file instead, without a
//	Content-Length and without range support.
//	HEAD is answered with the headers alone.
//
//	---
//	produces:
//	  - application/octet-stream
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the seed
//	    type: string
//	    required: true
//	  - in: path
//	    name: params
//	    description: |-
//	      Flat sequence of "<key>/<value>" pairs (architecture, type,
//	      optionally channel) followed by a filename segment, e.g.
//	      "architecture/x86_64/type/iso/file.iso" for the image the token seed
//	      resolves to, or "architecture/x86_64/type/iso/a1B2c3D4e5F6.iso" for
//	      one already generated image.
//	    type: string
//	    required: true
//	  - in: header
//	    name: Accept
//	    description: |-
//	      Set to "application/gzip" to get the gzip compressed image as a file,
//	      which is returned without range support.
//	    type: string
//	    required: false
//	  - in: header
//	    name: Range
//	    description: |-
//	      Byte range of the image to return, e.g. "bytes=1048576-".
//	    type: string
//	    required: false
//	  - in: header
//	    name: If-Range
//	    description: |-
//	      Last modification the client saw, as returned in "Last-Modified".
//	      If the image changed since, the complete image is returned instead
//	      of the requested range.
//	    type: string
//	    required: false
//	responses:
//	  "200":
//	    $ref: "#/responses/TokenSeedResponse"
//	  "206":
//	    $ref: "#/responses/TokenSeedResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "416":
//	    $ref: "#/responses/BadRequest"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (t *tokenHandler) tokenSeedImageGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	name := r.PathValue("name")

	imageType, architecture, channel, fingerprintID, err := parseSeedImageParams(r.PathValue("params"))
	if err != nil {
		return response.BadRequest(err)
	}

	if fingerprintID != "" {
		image, err := t.service.GetPreparedTokenSeedImage(r.Context(), UUID, name, imageType, architecture, channel, fingerprintID)
		if err != nil {
			return response.SmartError(err)
		}

		return response.ServeContentResponse(r, image.Content, image.Filename, image.ModTime, image.Size, nil)
	}

	seedConfig, err := t.service.GetTokenSeedByName(r.Context(), UUID, name)
	if err != nil {
		return response.SmartError(err)
	}

	if !seedConfig.Public {
		resp := checkPermission(t.authorizer, r, authz.ObjectTypeServer, authz.EntitlementCanView)
		if resp != nil {
			return resp
		}
	}

	filename := fmt.Sprintf("pre-seed-%s%s", name, imageType.FileExt())

	if response.RequestsGzipFile(r) && r.Header.Get("Range") == "" {
		rc, err := t.service.GetCompressedTokenImageFromTokenSeed(r.Context(), UUID, name, imageType, architecture, channel)
		if err != nil {
			return response.SmartError(err)
		}

		return response.ReadCloserResponse(r, rc, true, filename, -1, map[string]string{
			"Accept-Ranges": "none",
		})
	}

	image, err := t.service.GetSeekableTokenImageFromTokenSeed(r.Context(), UUID, name, imageType, architecture, channel)
	if err != nil {
		return response.SmartError(err)
	}

	return response.ServeContentResponse(r, image.Content, filename, image.ModTime, image.Size, nil)
}

// seedImageFingerprintIDRegexp matches the terminal filename segment naming an
// already generated image. The segment ends up addressing a file, so nothing
// but the shape provisioning.SeedImageFingerprintID produces is let through.
var seedImageFingerprintIDRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{12}$`)

// parseSeedImageParams parses the "/{params...}" tail of the token seed image
// route. params is a flat sequence of "<key>/<value>" pairs followed by a final
// filename segment.
//
// That last segment tells the two ways of asking for an image apart. A segment
// naming one already generated image is returned as fingerprintID.
func parseSeedImageParams(params string) (imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, fingerprintID string, err error) {
	segments := strings.Split(strings.Trim(params, "/"), "/")
	if len(segments) < 2 {
		return "", "", "", "", fmt.Errorf("Missing parameters in seed image path %q", params)
	}

	filename := segments[len(segments)-1]
	segments = segments[:len(segments)-1]

	basename := strings.TrimSuffix(filename, filepath.Ext(filename))
	if seedImageFingerprintIDRegexp.MatchString(basename) {
		fingerprintID = basename
	}

	if len(segments)%2 != 0 {
		return "", "", "", "", fmt.Errorf("Seed image path %q has an odd number of key/value segments", params)
	}

	values := map[string]string{}
	for i := 0; i < len(segments); i += 2 {
		key := segments[i]

		_, exists := values[key]
		if exists {
			return "", "", "", "", fmt.Errorf("Duplicate parameter %q in seed image path", key)
		}

		switch key {
		case "architecture", "channel", "type":
			values[key] = segments[i+1]

		default:
			return "", "", "", "", fmt.Errorf("Unknown parameter %q in seed image path", key)
		}
	}

	typeArg, ok := values["type"]
	if !ok {
		return "", "", "", "", fmt.Errorf("Missing required parameter %q in seed image path", "type")
	}

	imageType = api.ImageType(typeArg)
	if !imageType.IsValid() {
		return "", "", "", "", fmt.Errorf("Image type %q is not valid", typeArg)
	}

	architectureArg, ok := values["architecture"]
	if !ok {
		return "", "", "", "", fmt.Errorf("Missing required parameter %q in seed image path", "architecture")
	}

	architecture = images.UpdateFileArchitecture(architectureArg)

	_, ok = images.UpdateFileArchitectures[architecture]
	if !ok {
		return "", "", "", "", fmt.Errorf("Architecture %q is not valid", architectureArg)
	}

	return imageType, architecture, values["channel"], fingerprintID, nil
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
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the seed
//	    type: string
//	    required: true
//	  - in: body
//	    name: token
//	    description: Token seed config definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/TokenSeedPut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
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

	err = decodestrict.JSON(r.Body, &tokenSeed)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Failed to decode token seed: %w", err))
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
			Applications:     tokenSeed.Seeds.Applications,
			Install:          tokenSeed.Seeds.Install,
			MigrationManager: tokenSeed.Seeds.MigrationManager,
			Network:          tokenSeed.Seeds.Network,
			OperationsCenter: tokenSeed.Seeds.OperationsCenter,
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
//	parameters:
//	  - in: path
//	    name: uuid
//	    description: UUID of the token
//	    type: string
//	    format: uuid
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the seed
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
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
