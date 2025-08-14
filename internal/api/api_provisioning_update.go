package api

import (
	"archive/tar"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type updateHandler struct {
	service provisioning.UpdateService
}

func registerUpdateHandler(router Router, authorizer authz.Authorizer, service provisioning.UpdateService) {
	handler := &updateHandler{
		service: service,
	}

	// no authentication required for all GET routes
	router.HandleFunc("GET /{$}", response.With(handler.updatesGet))
	router.HandleFunc("GET /{uuid}", response.With(handler.updateGet))
	router.HandleFunc("GET /{uuid}/changelog", response.With(handler.updateChangelogGet))
	router.HandleFunc("GET /{uuid}/files", response.With(handler.updateFilesGet))
	router.HandleFunc("GET /{uuid}/files/{filename}", response.With(handler.updateFileGet))

	// authentication and authorization required to upload updates.
	router.HandleFunc("POST /{$}", response.With(handler.updatesPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("DELETE /{$}", response.With(handler.updatesDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /:refresh", response.With(handler.updatesRefreshPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
}

// swagger:operation GET /1.0/provisioning/updates updates updates_get
//
//	Get the updates
//
//	Returns a list of updates (URLs).
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: channel
//	    description: Channel to filter for.
//	    type: string
//	    example: stable
//	responses:
//	  "200":
//	    description: API updates
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
//	          description: List of updates
//                items:
//                  type: string
//                example: |-
//                  [
//                    "/1.0/provisioning/updates/b32d0079-c48b-4957-b1cb-bef54125c861",
//                    "/1.0/provisioning/updates/464d229b-3069-4a82-bc59-b215a7c6ed1b"
//                  ]
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/updates?recursion=1 updates updates_get_recursion
//
//	Get the updates
//
//	Returns a list of updates (structs) sorted by version.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: query
//	    name: channel
//	    description: Channel to filter for.
//	    type: string
//	    example: stable
//	  - in: query
//	    name: origin
//	    description: Origin to filter for.
//	    type: string
//	    example: images.linuxcontainers.org
//	  - in: query
//	    name: status
//	    description: Status to filter for.
//	    type: string
//	    example: ready
//	responses:
//	  "200":
//	    description: API updates
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
//	          description: List of updates
//	          items:
//	            $ref: "#/definitions/Update"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateHandler) updatesGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	var filter provisioning.UpdateFilter

	if r.URL.Query().Get("channel") != "" {
		filter.Channel = ptr.To(r.URL.Query().Get("channel"))
	}

	if r.URL.Query().Get("origin") != "" {
		filter.Origin = ptr.To(r.URL.Query().Get("origin"))
	}

	if r.URL.Query().Get("status") != "" {
		var status api.UpdateStatus
		err = status.UnmarshalText([]byte(r.URL.Query().Get("status")))
		if err != nil {
			return response.SmartError(fmt.Errorf("Invalid status"))
		}

		filter.Status = &status
	}

	if recursion == 1 {
		updates, err := u.service.GetAllWithFilter(r.Context(), filter)
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Update, 0, len(updates))
		for _, update := range updates {
			result = append(result, api.Update{
				UUID:        update.UUID,
				Version:     update.Version,
				PublishedAt: update.PublishedAt,
				Severity:    update.Severity,
				Origin:      update.Origin,
				URL:         update.URL,
				Channel:     update.Channel,
				Changelog:   update.Changelog,
				Status:      update.Status,
			})
		}

		return response.SyncResponse(true, result)
	}

	updateIDs, err := u.service.GetAllUUIDsWithFilter(r.Context(), filter)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(updateIDs))
	for _, id := range updateIDs {
		result = append(result, fmt.Sprintf("/%s/provisioning/updates/%s", api.APIVersion, id))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/updates updates updates_post
//
//	Add a update
//
//	Creates a new update.
//
//	---
//	consumes:
//	  - application/octet-stream
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/SyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateHandler) updatesPost(r *http.Request) response.Response {
	defer r.Body.Close()

	tr := tar.NewReader(r.Body)

	id, err := u.service.CreateFromArchive(r.Context(), tr)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/updates"+id.String())
}

// swagger:operation DELETE /1.0/provisioning/updates updates updates_delete
//
//	Remove all updates
//
//	Remove all updates and free up all disk space occupied by updates.
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
func (u *updateHandler) updatesDelete(r *http.Request) response.Response {
	err := u.service.CleanupAll(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/provisioning/updates/:refresh updates updates_refresh_post
//
//	Trigger a refresh of the updates
//
//	Refresh the updates provided by Operations Center.
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
func (u *updateHandler) updatesRefreshPost(r *http.Request) response.Response {
	err := u.service.Refresh(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/provisioning/updates/{uuid} updates update_get
//
//	Get the update
//
//	Gets a specific update.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Update
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
//	          $ref: "#/definitions/Update"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateHandler) updateGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	update, err := u.service.GetByUUID(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Update{
			UUID:        update.UUID,
			Version:     update.Version,
			PublishedAt: update.PublishedAt,
			Severity:    update.Severity,
			Origin:      update.Origin,
			URL:         update.URL,
			Channel:     update.Channel,
			Changelog:   update.Changelog,
			Status:      update.Status,
		},
		update,
	)
}

// swagger:operation GET /1.0/provisioning/updates/{uuid}/changelog updates update_changelog_get
//
//	Get the update changelog
//
//	Gets the changelog for a specific update.
//
//	---
//	produces:
//	  - text/plain
//	  - application/json
//	responses:
//	  "200":
//	    description: Update changelog
//	    content:
//	      text/plain:
//	        schema:
//	          type: string
//	          example: This is the changelog.
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateHandler) updateChangelogGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	update, err := u.service.GetByUUID(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponsePlain(
		true,
		false,
		update.Changelog,
	)
}

// swagger:operation GET /1.0/provisioning/updates/{uuid}/files updates updates_files_get
//
//	Get the update files
//
//	Returns a list of update files (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API update files
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
//	          description: List of update files
//	          items:
//	            $ref: "#/definitions/UpdateFile"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateHandler) updateFilesGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	updateFiles, err := u.service.GetUpdateAllFiles(r.Context(), UUID)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]api.UpdateFile, 0, len(updateFiles))
	for _, updateFile := range updateFiles {
		result = append(result, api.UpdateFile{
			Filename:     updateFile.Filename,
			Size:         updateFile.Size,
			Sha256:       updateFile.Sha256,
			Component:    updateFile.Component,
			Type:         updateFile.Type,
			Architecture: updateFile.Architecture,
		})
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/provisioning/updates/{uuid}/files/{filename} update update_file_get
//
//	Get the update file
//
//	Gets a specific update file.
//
//	---
//	produces:
//	  - application/octet-stream
//	responses:
//	  "200":
//	    description: Raw file data
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *updateHandler) updateFileGet(r *http.Request) response.Response {
	UUIDString := r.PathValue("uuid")
	filename := r.PathValue("filename")

	UUID, err := uuid.Parse(UUIDString)
	if err != nil {
		return response.BadRequest(err)
	}

	rc, fileSize, err := u.service.GetUpdateFileByFilename(r.Context(), UUID, filename)
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, rc, false, filename, fileSize, nil)
}
