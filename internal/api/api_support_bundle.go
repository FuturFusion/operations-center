package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/supportbundle"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type supportBundleHandler struct {
	service supportbundle.SupportBundleService
}

func registerSupportBundleHandler(router Router, authorizer *authz.Authorizer, service supportbundle.SupportBundleService) {
	handler := &supportBundleHandler{
		service: service,
	}

	router.HandleFunc("POST /{$}", response.With(handler.supportBundlePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug)))
}

// swagger:operation POST /1.0/support-bundle support-bundle support_bundle_post
//
//	Create a support bundle
//
//	Create and return a zip archive with diagnostic data of a cluster, a
//	project, an instance or a server. The data is collected live, a part,
//	which can not be collected, is replaced by an error file in the archive.
//	Secrets in the configuration are redacted, logs are included as is.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	  - application/zip
//	parameters:
//	  - in: body
//	    name: support_bundle_post
//	    description: Affected resource
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SupportBundlePost"
//	responses:
//	  "200":
//	    description: zip archive
//	    schema:
//	      type: string
//	      format: binary
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *supportBundleHandler) supportBundlePost(r *http.Request) response.Response {
	var request api.SupportBundlePost

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		return response.BadRequest(err)
	}

	filename, rc, err := s.service.Create(r.Context(), request)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to create support bundle: %w", err))
	}

	return response.ReadCloserResponse(r, rc, false, filename, -1, map[string]string{
		"Content-Type": "application/zip",
	})
}
