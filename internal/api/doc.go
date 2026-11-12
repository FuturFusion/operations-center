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
// Every POST request needs a Content-Type header. A PUT, PATCH or DELETE
// request needs it, if the request has a body. The server rejects the media
// types "text/plain", "application/x-www-form-urlencoded" and
// "multipart/form-data" with status 415. The only exception is the upload of
// an Incus image, which uses "multipart/form-data". Browsers send these media
// types to other sites without asking the server first. The check protects
// against cross-site request forgery. Requests through the local unix socket
// are not checked.
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
