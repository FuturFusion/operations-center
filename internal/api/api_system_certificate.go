package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/shared/api"
)

type systemHandler struct {
	service system.SystemService
}

func registerSystemHandler(router Router, authorizer authz.Authorizer, service system.SystemService) {
	handler := &systemHandler{
		service: service,
	}

	router.HandleFunc("PUT /certificate", response.With(handler.certificatePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation PUT /1.0/system/certificate system system_certificate_put
//
//	Update the system's certificate and key
//
//	Update the system's certificate and key.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: system_certificate_put
//	    description: System certificate definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SystemCertificatePut"
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: System certificate update response
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
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) certificatePut(r *http.Request) response.Response {
	var request api.SystemCertificatePut

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateCertificate(r.Context(), request.Certificate, request.Key)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update system certificate: %w", err))
	}

	return response.EmptySyncResponse
}
