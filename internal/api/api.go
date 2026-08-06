package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/util/response"
)

// swagger:operation GET / server api_get
//
//	Get the supported API endpoints
//
//	Returns a list of supported API versions (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/APIEndpointsResponse"
func rootHandler(_ *http.Request) response.Response {
	return response.SyncResponse(true, []string{"/1.0"})
}
