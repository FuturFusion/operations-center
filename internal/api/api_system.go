package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/response"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
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
	router.HandleFunc("POST /:clean-cache", response.With(handler.cleanCachePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
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
//	      $ref: "#/definitions/CertificatePost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) certificatePost(r *http.Request) response.Response {
	var request apisystem.CertificatePost

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
//	    $ref: "#/responses/EmptySyncResponse"
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

// swagger:operation GET /1.0/system/network system system_network_get
//
//	Get the system's network configuration
//
//	Get the system's network configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/SystemNetworkResponse"
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

// swagger:operation PUT /1.0/system/network system system_network_put
//
//	Update the system's network configuration
//
//	Update the system's network configuration.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: system_network_put
//	    description: System network configuration definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/NetworkPut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) networkPut(r *http.Request) response.Response {
	var networkConfig apisystem.NetworkPut

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

// swagger:operation GET /1.0/system/security system system_security_get
//
//	Get the system's security configuration
//
//	Get the system's security configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/SystemSecurityResponse"
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
//	      $ref: "#/definitions/SecurityPut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) securityPut(r *http.Request) response.Response {
	var securityConfig apisystem.SecurityPut

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

// swagger:operation GET /1.0/system/settings system system_settings_get
//
//	Get the system's settings configuration
//
//	Get the system's settings configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/SystemSettingsResponse"
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
//	      $ref: "#/definitions/SettingsPut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) settingsPut(r *http.Request) response.Response {
	var settingsConfig apisystem.SettingsPut

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

// swagger:operation GET /1.0/system/updates system system_updates_get
//
//	Get the system's updates configuration
//
//	Get the system's updates configuration.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/SystemUpdatesResponse"
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
//	      $ref: "#/definitions/UpdatesPut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) updatesPut(r *http.Request) response.Response {
	var updatesConfig apisystem.UpdatesPut

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

// swagger:operation POST /1.0/system/:clean-cache system system_clean_cache_post
//
//	Clean operations-center's cache
//
//	Remove reproducible, cached content.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *systemHandler) cleanCachePost(r *http.Request) response.Response {
	err := s.service.CleanCache(r.Context())
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to clean operations-center's cache: %w", err))
	}

	return response.EmptySyncResponse
}
