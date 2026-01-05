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

func registerSystemHandler(router Router, authorizer *authz.Authorizer, service system.SystemService) {
	handler := &systemHandler{
		service: service,
	}

	router.HandleFunc("POST /certificate", response.With(handler.certificatePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /certificate/:renew", response.With(handler.certificateRenewPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /network", response.With(handler.networkGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /network", response.With(handler.networkPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /security", response.With(handler.securityGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /security", response.With(handler.securityPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /settings", response.With(handler.settingsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /settings", response.With(handler.settingsPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /updates", response.With(handler.updatesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /updates", response.With(handler.updatesPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation POST /1.0/system/certificate system system_certificate_post
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
//	    name: system_certificate_post
//	    description: System certificate definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SystemCertificatePost"
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
func (s *systemHandler) certificatePost(r *http.Request) response.Response {
	var request api.SystemCertificatePost

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

// swagger:operation POST /1.0/system/certificate/:renew system system_certificate_renew_post
//
//	Renew the system's ACME certificate
//
//	Renew the system's ACME certificate.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: System certificate renew response
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
func (s *systemHandler) certificateRenewPost(r *http.Request) response.Response {
	_, err := s.service.TriggerCertificateRenew(r.Context(), true)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to renew system certificate: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation Get /1.0/system/network system system_network_get
//
//	Update the system's network configuration
//
//	Update the system's network configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API system network configuration
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
//	          type: object
//	          description: Configuration object
//	          items:
//	            $ref: "#/definitions/SystemNetwork"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) networkGet(r *http.Request) response.Response {
	networkConfig := s.service.GetNetworkConfig(r.Context())
	return response.SyncResponse(true, networkConfig)
}

// swagger:operation PUT /1.0/system/config system system_config_put
//
//	Update the system's configuration
//
//	Update the system's configuration.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: system_configuration_put
//	    description: System configuration definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SystemConfigurationPut"
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: System configuration update response
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
func (s *systemHandler) networkPut(r *http.Request) response.Response {
	var networkConfig api.SystemNetworkPut

	err := json.NewDecoder(r.Body).Decode(&networkConfig)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateNetworkConfig(r.Context(), networkConfig)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update network configuration: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation Get /1.0/system/security system system_security_get
//
//	Update the system's security configuration
//
//	Update the system's security configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API security configuration
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
//	          type: object
//	          description: Security configuration object
//	          items:
//	            $ref: "#/definitions/SystemSecurityConfig"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) securityGet(r *http.Request) response.Response {
	securityConfig := s.service.GetSecurityConfig(r.Context())
	return response.SyncResponse(true, securityConfig)
}

// swagger:operation PUT /1.0/system/security system system_security_put
//
//	Update the system's security configuration
//
//	Update the system's security configuration.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: system_security_put
//	    description: System security configuration definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SystemSecurityConfigPut"
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: System security configuration update response
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
func (s *systemHandler) securityPut(r *http.Request) response.Response {
	var securityConfig api.SystemSecurityPut

	err := json.NewDecoder(r.Body).Decode(&securityConfig)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateSecurityConfig(r.Context(), securityConfig)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update security configuration: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation Get /1.0/system/settings system system_settings_get
//
//	Update the system's settings configuration
//
//	Update the system's settings configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API settings configuration
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
//	          type: object
//	          description: Settings configuration object
//	          items:
//	            $ref: "#/definitions/SystemSettingsConfig"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) settingsGet(r *http.Request) response.Response {
	settingsConfig := s.service.GetSettingsConfig(r.Context())
	return response.SyncResponse(true, settingsConfig)
}

// swagger:operation PUT /1.0/system/settings system system_settings_put
//
//	Update the system's settings configuration
//
//	Update the system's settings configuration.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: system_settings_put
//	    description: System settings configuration definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SystemSettingsConfigPut"
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: System settings configuration update response
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
func (s *systemHandler) settingsPut(r *http.Request) response.Response {
	var settingsConfig api.SystemSettingsPut

	err := json.NewDecoder(r.Body).Decode(&settingsConfig)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateSettingsConfig(r.Context(), settingsConfig)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update settings configuration: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation Get /1.0/system/updates system system_updates_get
//
//	Update the system's updates configuration
//
//	Update the system's updates configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API updates configuration
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
//	          type: object
//	          description: Updates configuration object
//	          items:
//	            $ref: "#/definitions/SystemUpdatesConfig"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) updatesGet(r *http.Request) response.Response {
	updatesConfig := s.service.GetUpdatesConfig(r.Context())
	return response.SyncResponse(true, updatesConfig)
}

// swagger:operation PUT /1.0/system/updates system system_updates_put
//
//	Update the system's updates configuration
//
//	Update the system's updates configuration.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: system_updates_put
//	    description: System security configuration definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/SystemUpdatesConfigPut"
//	responses:
//	  "200":
//	    description: Empty response
//	    schema:
//	      type: object
//	      description: System updates configuration update response
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
func (s *systemHandler) updatesPut(r *http.Request) response.Response {
	var updatesConfig api.SystemUpdatesPut

	err := json.NewDecoder(r.Body).Decode(&updatesConfig)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateUpdatesConfig(r.Context(), updatesConfig)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update updates configuration: %w", err))
	}

	return response.EmptySyncResponse
}
