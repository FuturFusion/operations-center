// Operations Center API
//
// Operations Center is the central overview of an Incus deployment. It acts as
// the registration point for all servers running IncusOS, handles update
// tracking and rollout, provisions new Incus clusters and keeps track of
// resources across all clusters.
//
// The API is served over HTTPS. Every response is a JSON object carrying a
// "type" field, which is "sync" for the responses documented here and "error"
// for failures. The payload of a successful response is carried in "metadata".
//
// Clients authenticate either with a TLS client certificate, which is
// negotiated at the transport level and therefore cannot be expressed in
// Swagger 2.0, or with an OIDC bearer token, which is described by the "oidc"
// security definition.
//
//	Schemes: https
//	BasePath: /
//	Version: 1.0
//	License: Apache-2.0 https://www.apache.org/licenses/LICENSE-2.0
//
//	Consumes:
//	  - application/json
//
//	Produces:
//	  - application/json
//
//	SecurityDefinitions:
//	  oidc:
//	    type: apiKey
//	    name: Authorization
//	    in: header
//	    description: OIDC bearer token, sent as "Bearer <token>".
//
// swagger:meta
package api
