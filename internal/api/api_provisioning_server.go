package api

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/domain"
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

func registerProvisioningServerHandler(router Router, authorizer *authz.Authorizer, service provisioning.ServerService, clientCertificate string) {
	handler := &serverHandler{
		service:           service,
		clientCertificate: clientCertificate,
	}

	// Creating new servers (POST requests for servers) is authenticated using
	// a token. Therefore no authorization is performed for these requests.
	router.HandleFunc("POST /{$}", response.With(handler.serversPost))

	// Self update of existing servers (PUT request of a server for their own record)
	// is authenticated using the stored certificate of the server or by using
	// the unix socket connection. Therefore no authorization is performed for
	// these requests.
	//
	// Using the unix socket connection for this end point is a special case,
	// since it only allows to update the server record of this Operations Center
	// instance.
	router.HandleFunc("PUT /:self", response.With(handler.serverPutSelf))

	// Self registering of IncusOS which this Operations Center instance is served
	// from (POST by IncusOS serving Operations Center for its own record).
	// This route is only available through unix socket, therefore no
	// authentication and authorization is performed for these requests.
	router.HandleFunc("POST /:self_register", response.With(handler.serverPostSelfRegister))

	router.HandleFunc("GET /{$}", response.With(handler.serversGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{name}", response.With(handler.serverGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.serverPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.serverDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}", response.With(handler.serverPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/:resync", response.With(handler.serverResyncPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/system/:evacuate", response.With(handler.serverSystemEvacuatePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/system/:poweroff", response.With(handler.serverSystemPoweroffPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/system/:reboot", response.With(handler.serverSystemRebootPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("POST /{name}/system/:update", response.With(handler.serverSystemUpdatePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /{name}/system/network", response.With(handler.serverSystemNetworkGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}/system/network", response.With(handler.serverSystemNetworkPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /{name}/system/storage", response.With(handler.serverSystemStorageGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}/system/storage", response.With(handler.serverSystemStoragePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /{name}/system/update", response.With(handler.serverSystemUpdateGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}/system/update", response.With(handler.serverSystemUpdatePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
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
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/provisioning/servers/one",
//	              "/1.0/provisioning/servers/two"
//	            ]
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
//	    name: status
//	    description: Status to filter for.
//	    type: string
//	    example: ready
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

	if r.URL.Query().Get("status") != "" {
		var status api.ServerStatus
		err = status.UnmarshalText([]byte(r.URL.Query().Get("status")))
		if err != nil {
			return response.SmartError(fmt.Errorf("Invalid status"))
		}

		filter.Status = &status
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
				ServerPost: api.ServerPost{
					Name:          server.Name,
					ConnectionURL: server.ConnectionURL,
					ServerPut: api.ServerPut{
						PublicConnectionURL: server.PublicConnectionURL,
						Channel:             server.Channel,
					},
				},
				Certificate:  server.Certificate,
				Fingerprint:  server.Fingerprint,
				Cluster:      ptr.From(server.Cluster),
				Type:         server.Type,
				HardwareData: server.HardwareData,
				OSData:       server.OSData,
				VersionData:  server.VersionData,
				Status:       server.Status,
				LastUpdated:  server.LastUpdated,
				LastSeen:     server.LastSeen,
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

	var server api.ServerPost

	// Decode into the new server.
	err = json.NewDecoder(r.Body).Decode(&server)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Request decoding: %v", err))
	}

	// Ensure presence of client certificate.
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return response.BadRequest(fmt.Errorf("No client certificate provided"))
	}

	// Encode client certificate in pem format
	certificate := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: r.TLS.PeerCertificates[0].Raw,
	})

	_, err = s.service.Create(r.Context(), token, provisioning.Server{
		Name:                server.Name,
		ConnectionURL:       server.ConnectionURL,
		PublicConnectionURL: server.PublicConnectionURL,
		Certificate:         string(certificate),
		Channel:             server.Channel,
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
			ServerPost: api.ServerPost{
				Name:          server.Name,
				ConnectionURL: server.ConnectionURL,
				ServerPut: api.ServerPut{
					PublicConnectionURL: server.PublicConnectionURL,
					Channel:             server.Channel,
				},
			},
			Certificate:  server.Certificate,
			Fingerprint:  server.Fingerprint,
			Cluster:      ptr.From(server.Cluster),
			Type:         server.Type,
			HardwareData: server.HardwareData,
			OSData:       server.OSData,
			VersionData:  server.VersionData,
			Status:       server.Status,
			LastUpdated:  server.LastUpdated,
			LastSeen:     server.LastSeen,
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
//	      $ref: "#/definitions/ServerPut"
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

	var server api.ServerPut

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

	currentServer.PublicConnectionURL = server.PublicConnectionURL

	// Only allow changing of Channel, if server is not clustered. Otherwise
	// the change of the channel needs to happen through the cluster.
	var updateServer bool
	if currentServer.Cluster == nil {
		// Only trigger update of server, when the channel, the server is following,
		// has changed.
		updateServer = currentServer.Channel != server.Channel
		currentServer.Channel = server.Channel
	}

	err = s.service.Update(ctx, *currentServer, updateServer)
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
//	Special case is, if this endpoint is called over the unix socket. In this
//	case, it allows to update the server record of this Operations Center
//	itself.
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
	// Self update through unix socket from IncusOS serving Operations Center.
	if r.RemoteAddr == "@" && r.TLS == nil {
		return s.serverPutSelfUnixSocket(r)
	}

	// Ensure presence of client certificate.
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
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

func (s *serverHandler) serverPutSelfUnixSocket(r *http.Request) response.Response {
	var serverUpdate api.ServerSelfUpdate
	err := json.NewDecoder(r.Body).Decode(&serverUpdate)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.SelfUpdate(r.Context(), provisioning.ServerSelfUpdate{
		ConnectionURL: serverUpdate.ConnectionURL,
		Self:          true,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed self-updating own Operations Center over unix socket: %w", err))
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/:self_register servers server_post_self_register
//
//	Register an Operations Center server by it self onto it self
//
//	Register an Operations Center server by it self onto it self.
//	Authentication is done through unix socket.
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
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverPostSelfRegister(r *http.Request) response.Response {
	// Self update through unix socket from IncusOS serving Operations Center.
	if r.RemoteAddr != "@" || r.TLS != nil {
		return response.Forbidden(fmt.Errorf("Self register is only possible through unix socket"))
	}

	err := s.service.SelfRegisterOperationsCenter(r.Context())
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

// swagger:operation POST /1.0/provisioning/servers/{name}/:resync servers server_resync_post
//
//	Sync server state
//
//	Trigger re-sync of the server's state.
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
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverResyncPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := s.service.ResyncByName(r.Context(), "", domain.LifecycleEvent{
		ResourceType: domain.ResourceTypeServer,
		Operation:    domain.LifecycleOperationUpdate,
		Source: domain.LifecycleSource{
			Name: name,
		},
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to resync server %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/{name}/system/:evacuate servers_system_evacuate server_system_evacuate_post
//
//	Evacuate server
//
//	Triggers an evacuate operation on the server.
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
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemEvacuatePost(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := s.service.EvacuateSystemByName(r.Context(), name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to evacuate server %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/{name}/system/:poweroff servers_system_poweroff server_system_poweroff_post
//
//	Poweroff server
//
//	Triggers a poweroff operation on the server.
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
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemPoweroffPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := s.service.PoweroffSystemByName(r.Context(), name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to poweroff server %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/{name}/system/:reboot servers_system_reboot server_system_reboot_post
//
//	Reboot server
//
//	Triggers a reboot operation on the server.
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
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemRebootPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := s.service.RebootSystemByName(r.Context(), name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to reboot server %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/{name}/system/:update servers_system_update server_system_update_post
//
//	Update server
//
//	Triggers an update operation on the server.
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
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemUpdatePost(r *http.Request) response.Response {
	name := r.PathValue("name")

	var updateRequest api.ServerUpdatePost

	err := json.NewDecoder(r.Body).Decode(&updateRequest)
	if err != nil {
		return response.BadRequest(fmt.Errorf("Request decoding: %v", err))
	}

	err = s.service.UpdateSystemByName(r.Context(), name, updateRequest)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update server %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/provisioning/servers/{name}/system/network servers_system_network server_system_network_get
//
//	Get server network configuration
//
//	Gets the network configuration of a specific server.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Server network
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
//	          $ref: "#/definitions/ServerSystemNetworkConfig"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemNetworkGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	server, err := s.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		server.OSData.Network,
		server.OSData.Network,
	)
}

// swagger:operation PUT /1.0/provisioning/servers/{name}/system/network servers_system_network server_system_network_put
//
//	Update server network configuration
//
//	Updates the network configuration of a specific server.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server network configuration
//	    description: Server network configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ServerSystemNetworkConfig"
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
func (s *serverHandler) serverSystemNetworkPut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var systemNetwork api.ServerSystemNetwork

	err := json.NewDecoder(r.Body).Decode(&systemNetwork)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateSystemNetwork(r.Context(), name, systemNetwork)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update server network configuration for %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/provisioning/servers/{name}/system/storage servers_system_storage server_system_storage_get
//
//	Get server storage configuration
//
//	Gets the storage configuration of a specific server.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Server storage
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
//	          $ref: "#/definitions/ServerSystemStorageConfig"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemStorageGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	server, err := s.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		server.OSData.Storage,
		server.OSData.Storage,
	)
}

// swagger:operation PUT /1.0/provisioning/servers/{name}/system/storage servers_system_storage server_system_storage_put
//
//	Update server storage configuration
//
//	Updates the storage configuration of a specific server.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server storage configuration
//	    description: Server storage configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ServerSystemStorageConfig"
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
func (s *serverHandler) serverSystemStoragePut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var systemStorage api.ServerSystemStorage

	err := json.NewDecoder(r.Body).Decode(&systemStorage)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateSystemStorage(r.Context(), name, systemStorage)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update server storage configuration for %q: %w", name, err))
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/provisioning/servers/{name}/system/update servers_system_update server_system_update_get
//
//	Get server update configuration
//
//	Gets the update configuration of a specific server.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Server update
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
//	          $ref: "#/definitions/ServerSystemUpdateConfig"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serverSystemUpdateGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	server, err := s.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	// FIXME: What should we return here? What should we use for the ETag?

	return response.SyncResponseETag(
		true,
		server.VersionData,
		server.VersionData,
	)
}

// swagger:operation PUT /1.0/provisioning/servers/{name}/system/update servers_system_update server_system_update_put
//
//	Update server update configuration
//
//	Updates the update configuration of a specific server.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: server update configuration
//	    description: Server update configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ServerSystemUpdateConfig"
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
func (s *serverHandler) serverSystemUpdatePut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var systemUpdate api.ServerSystemUpdate

	err := json.NewDecoder(r.Body).Decode(&systemUpdate)
	if err != nil {
		return response.BadRequest(err)
	}

	err = s.service.UpdateSystemUpdate(r.Context(), name, systemUpdate)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to update server update configuration for %q: %w", name, err))
	}

	return response.EmptySyncResponse
}
