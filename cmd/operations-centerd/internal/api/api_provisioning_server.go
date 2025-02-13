package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type serverHandler struct {
	service provisioning.ServerService
}

func registerProvisioningServerHandler(router *http.ServeMux, service provisioning.ServerService) {
	handler := &serverHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.serversGet))
	router.HandleFunc("POST /{$}", response.With(handler.serversPost))
	router.HandleFunc("GET /{hostname}", response.With(handler.serverGet))
	router.HandleFunc("PUT /{hostname}", response.With(handler.serverPut))
	router.HandleFunc("DELETE /{hostname}", response.With(handler.serverDelete))
	router.HandleFunc("POST /{hostname}", response.With(handler.serverPost))
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

	if recursion == 1 {
		servers, err := s.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Server, 0, len(servers))
		for _, server := range servers {
			result = append(result, api.Server{
				ID:            server.ID,
				ClusterID:     server.ClusterID,
				Hostname:      server.Hostname,
				Type:          server.Type,
				ConnectionURL: server.ConnectionURL,
				HardwareData:  server.HardwareData,
				VersionData:   server.VersionData,
				LastUpdated:   server.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	serverHostnames, err := s.service.GetAllHostnames(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(serverHostnames))
	for _, hostname := range serverHostnames {
		result = append(result, fmt.Sprintf("/%s/servers/%s", api.APIVersion, hostname))
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
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *serverHandler) serversPost(r *http.Request) response.Response {
	var server api.Server

	// Decode into the new server.
	err := json.NewDecoder(r.Body).Decode(&server)
	if err != nil {
		return response.BadRequest(err)
	}

	_, err = s.service.Create(r.Context(), provisioning.Server{
		ID:            server.ID,
		ClusterID:     server.ClusterID,
		Hostname:      server.Hostname,
		Type:          server.Type,
		ConnectionURL: server.ConnectionURL,
		HardwareData:  server.HardwareData,
		VersionData:   server.VersionData,
		LastUpdated:   server.LastUpdated,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating server: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/servers/"+server.Hostname)
}

// swagger:operation GET /1.0/provisioning/servers/{hostname} servers server_get
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
	hostname := r.PathValue("hostname")

	server, err := s.service.GetByHostname(r.Context(), hostname)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Server{
			ID:            server.ID,
			ClusterID:     server.ClusterID,
			Hostname:      server.Hostname,
			Type:          server.Type,
			ConnectionURL: server.ConnectionURL,
			HardwareData:  server.HardwareData,
			VersionData:   server.VersionData,
			LastUpdated:   server.LastUpdated,
		},
		server,
	)
}

// swagger:operation PUT /1.0/provisioning/servers/{hostname} servers server_put
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
	hostname := r.PathValue("hostname")

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

	currentServer, err := s.service.GetByHostname(ctx, hostname)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get server %q: %w", hostname, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentServer)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	_, err = s.service.UpdateByHostname(ctx, hostname, provisioning.Server{
		ID:            server.ID,
		ClusterID:     server.ClusterID,
		Hostname:      server.Hostname,
		Type:          server.Type,
		ConnectionURL: server.ConnectionURL,
		HardwareData:  server.HardwareData,
		VersionData:   server.VersionData,
		LastUpdated:   server.LastUpdated,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating server %q: %w", hostname, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/servers/"+hostname)
}

// swagger:operation DELETE /1.0/provisioning/servers/{hostname} servers server_delete
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
	hostname := r.PathValue("hostname")

	err := s.service.DeleteByHostname(r.Context(), hostname)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/servers/{hostname} servers server_post
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
	hostname := r.PathValue("hostname")

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

	currentServer, err := s.service.GetByHostname(ctx, hostname)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get server %q: %w", hostname, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentServer)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	_, err = s.service.RenameByHostname(ctx, hostname, provisioning.Server{
		ID:            server.ID,
		ClusterID:     server.ClusterID,
		Hostname:      server.Hostname,
		Type:          server.Type,
		ConnectionURL: server.ConnectionURL,
		HardwareData:  server.HardwareData,
		VersionData:   server.VersionData,
		LastUpdated:   server.LastUpdated,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed renaming server %q: %w", hostname, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/servers/"+server.Hostname)
}
