package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type tokenHandler struct {
	service operations.TokenService
}

func registerTokenHandler(router *http.ServeMux, service operations.TokenService) {
	handler := &tokenHandler{
		service: service,
	}

	router.HandleFunc("POST /{$}",
		response.With(
			handler.tokensPost,
		),
	)
}

// swagger:operation POST /1.0/tokens tokens tokens_post
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
//	      $ref: "#/definitions/Token"
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
	var token api.Token

	// Decode into the new token.
	err := json.NewDecoder(r.Body).Decode(&token)
	if err != nil {
		return response.BadRequest(err)
	}

	_, err = t.service.Create(r.Context(), operations.Token{
		UsesRemaining: token.UsesRemaining,
		ExpireAt:      token.ExpireAt,
		Description:   token.Description,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating token: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/tokens/"+token.UUID.String())
}
