package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/internal/version"
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
//	    $ref: "#/responses/ServerUntrustedResponse"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func api10Get(r *http.Request) response.Response {
	srv := api.ServerUntrusted{
		APIStatus:     api.APIStatus,
		APIVersion:    api.APIVersion,
		Auth:          api.AuthenticationUntrusted,
		AuthMethods:   []string{api.AuthenticationMethodOIDC, api.AuthenticationMethodTLS},
		ServerVersion: version.Version,
	}

	// Return the authentication protocol, if any, the client has been authenticated with.
	ctx := r.Context()

	authenticated, authenticatedOK := ctx.Value(authn.CtxAuthenticated).(bool)
	protocol, protocolOK := ctx.Value(authn.CtxProtocol).(string)

	if authenticatedOK && authenticated && protocolOK && protocol != "" {
		srv.Auth = protocol
	}

	return response.SyncResponseETag(true, srv, nil)
}
