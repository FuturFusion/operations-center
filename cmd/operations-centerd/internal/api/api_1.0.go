package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authn"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

func registerAPI10Handler(router Router) {
	router.HandleFunc("GET /{$}", response.With(api10Get))
}

// swagger:operation GET /1.0 server server_get_untrusted
//
//	Get the server environment
//
//	Shows a small subset of the server environment and configuration
//	which is required by untrusted clients to reach a server.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Server environment and configuration
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
//	          $ref: "#/definitions/ServerUntrusted"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func api10Get(r *http.Request) response.Response {
	srv := api.ServerUntrusted{
		APIStatus:   api.APIStatus,
		APIVersion:  api.APIVersion,
		Auth:        "untrusted",
		AuthMethods: []string{"oidc", "tls"},
	}

	// Return the authentication method, if any, that the client is using.
	ctx := r.Context()
	auth := ctx.Value(authn.CtxProtocol)
	if auth != nil {
		v, ok := auth.(string)
		if ok {
			srv.Auth = v
		}
	}

	return response.SyncResponseETag(true, srv, nil)
}
