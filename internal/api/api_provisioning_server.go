package api

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type serverHandler struct {
	service           provisioning.ServerService
	clientCertificate string
}

func registerProvisioningServerHandler(router Router, authorizer authz.Authorizer, service provisioning.ServerService, clientCertificate string) {
	handler := &serverHandler{
		service:           service,
		clientCertificate: clientCertificate,
	}

	// Creating new servers (POST requests for servers) is authenticated using
	// a token. Therefore no authorization is performed for these requests.
	router.HandleFunc("POST /{$}", response.With(handler.serversPost))

	// Self update of existing servers (PUT request of a server for their own record)
	// is authenticated using the stored certificate of the server. Therefore no
	// authorization is performed for these requests.
	router.HandleFunc("PUT /:self", response.With(handler.serverPutSelf))

	router.HandleFunc("GET /{$}", response.With(handler.serversGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{name}", response.With(handler.serverGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.serverPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.serverDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}", response.With(handler.serverPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/provisioning/servers servers servers_get
//
//	Get the servers
//
//	Returns a list of servers (URLs).
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: cluster
//	    description: Cluster name
//	    type: string
//	    example: cluster
//	  - in: query
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API servers
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
//	          type: array
//	          description: List of servers
//                items:
//                  type: string
//                example: |-
//                  [
//                    "/1.0/provisioning/servers/one",
//                    "/1.0/provisioning/servers/two"
//                  ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/servers?recursion=1 servers servers_get_recursion
//
//	Get the servers
//
//	Returns a list of servers (structs).
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: cluster
//	    description: Cluster name
//	    type: string
//	    example: cluster
//	  - in: query
//	    name: filter
//	    description: Filter expression
//	    type: string
//	    example: name == "value"
//	responses:
//	  "200":
//	    description: API servers
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
//	          type: array
//	          description: List of servers
//	          items:
//	            $ref: "#/definitions/Server"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serversGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	var filter provisioning.ServerFilter

	if r.URL.Query().Get("cluster") != "" {
		filter.Cluster = ptr.To(r.URL.Query().Get("cluster"))
	}

	if r.URL.Query().Get("filter") != "" {
		filter.Expression = ptr.To(r.URL.Query().Get("filter"))
	}

	if recursion == 1 {
		servers, err := s.service.GetAllWithFilter(r.Context(), filter)
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Server, 0, len(servers))
		for _, server := range servers {
			result = append(result, api.Server{
				Cluster:       ptr.From(server.Cluster),
				Name:          server.Name,
				Type:          server.Type,
				ConnectionURL: server.ConnectionURL,
				HardwareData:  server.HardwareData,
				OSData:        server.OSData,
				VersionData:   server.VersionData,
				Status:        server.Status,
				LastUpdated:   server.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	serverNames, err := s.service.GetAllNamesWithFilter(r.Context(), filter)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(serverNames))
	for _, name := range serverNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/servers/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/servers servers servers_post
//
//	Add a server
//
//	Creates a new server.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server
//	    description: Server configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/Server"
//	responses:
//	  "200":
//	    description: Register server response
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
//	          type: array
//	          description: Resgister server response details
//	          items:
//	            $ref: "#/definitions/ServerRegistrationResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serversPost(r *http.Request) response.Response {
	// Parse the token.
	tokenParam := r.URL.Query().Get("token")
	if tokenParam == "" {
		return response.BadRequest(fmt.Errorf("Missing token"))
	}

	token, err := uuid.Parse(tokenParam)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Invalid token: %v", err))
	}

	var server api.Server

	// Decode into the new server.
	err = json.NewDecoder(r.Body).Decode(&server)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Request decoding: %v", err))
	}

	// Ensure presence of client certificate.
	if len(r.TLS.PeerCertificates) == 0 {
		return response.BadRequest(fmt.Errorf("No client certificate provided"))
	}

	// Encode client certificate in pem format
	certificate := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: r.TLS.PeerCertificates[0].Raw,
	})

	_, err = s.service.Create(r.Context(), token, provisioning.Server{
		Name:          server.Name,
		ConnectionURL: server.ConnectionURL,
		Certificate:   string(certificate),
	})
	if err != nil {
		return response.Forbidden(fmt.Errorf("Failed creating server: %w", err))
	}

	result := api.ServerRegistrationResponse{
		ClientCertificate: s.clientCertificate,
	}

	return response.SyncResponseLocation(true, result, "/"+api.APIVersion+"/provisioning/servers/"+server.Name)
}

// swagger:operation GET /1.0/provisioning/servers/{name} servers server_get
//
//	Get the server
//
//	Gets a specific server.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Server
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
//	          $ref: "#/definitions/Server"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	server, err := s.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Server{
			Cluster:       ptr.From(server.Cluster),
			Name:          server.Name,
			Type:          server.Type,
			ConnectionURL: server.ConnectionURL,
			HardwareData:  server.HardwareData,
			OSData:        server.OSData,
			VersionData:   server.VersionData,
			Status:        server.Status,
			LastUpdated:   server.LastUpdated,
		},
		server,
	)
}

// swagger:operation PUT /1.0/provisioning/servers/{name} servers server_put
//
//	Update the server
//
//	Updates the server definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server
//	    description: Server definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/Server"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverPut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var server api.Server

	err := json.NewDecoder(r.Body).Decode(&server)
	if err != nil {
		return response.BadRequest(err)
	}

	ctx, trans := transaction.Begin(r.Context())
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			response.SmartError(fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err))
		}
	}()

	currentServer, err := s.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get server %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentServer)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = s.service.Update(ctx, provisioning.Server{
		Cluster:       ptr.To(server.Cluster),
		Name:          server.Name,
		Type:          server.Type,
		ConnectionURL: server.ConnectionURL,
		// TODO: Preservation of Certificate and Hardware Data should be part of service logic and not API handler
		Certificate:  currentServer.Certificate,
		HardwareData: currentServer.HardwareData,
		VersionData:  server.VersionData,
		Status:       server.Status,
		LastUpdated:  server.LastUpdated,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating server %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation PUT /1.0/provisioning/servers/:self servers server_put_self
//
//	Update of a server by it self
//
//	Update of a server definition by the server it self.
//	Authentication is done by the servers certificate provided during the
//	initial registration.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server
//	    description: Server definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ServerSelfUpdate"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverPutSelf(r *http.Request) response.Response {
	// Ensure presence of client certificate.
	if len(r.TLS.PeerCertificates) == 0 {
		return response.Forbidden(fmt.Errorf("No client certificate provided"))
	}

	var serverUpdate api.ServerSelfUpdate
	err := json.NewDecoder(r.Body).Decode(&serverUpdate)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.SelfUpdate(r.Context(), provisioning.ServerSelfUpdate{
		ConnectionURL:             serverUpdate.ConnectionURL,
		AuthenticationCertificate: r.TLS.PeerCertificates[0],
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed self-updating from server %q: %w", r.RemoteAddr, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation DELETE /1.0/provisioning/servers/{name} servers server_delete
//
//	Delete the server
//
//	Removes the server.
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
func (s *serverHandler) serverDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := s.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/{name} servers server_post
//
//	Rename the server
//
//	Renames the server.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server
//	    description: Server definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/Server"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	var server api.Server

	err := json.NewDecoder(r.Body).Decode(&server)
	if err != nil {
		return response.BadRequest(err)
	}

	ctx, trans := transaction.Begin(r.Context())
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			response.SmartError(fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err))
		}
	}()

	currentServer, err := s.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get server %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentServer)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	err = s.service.Rename(ctx, name, server.Name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed renaming server %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/servers/"+server.Name)
}
