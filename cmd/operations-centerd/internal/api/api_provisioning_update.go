package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type updateHandler struct {
	service provisioning.UpdateService
}

func registerUpdateHandler(router Router, service provisioning.UpdateService) {
	handler := &updateHandler{
		service: service,
	}

	// no authentication required for all routes
	router.HandleFunc("GET /{$}", response.With(handler.updatesGet))
	router.HandleFunc("GET /{uuid}", response.With(handler.updateGet))
	router.HandleFunc("GET /{uuid}/files", response.With(handler.updateFilesGet))
	router.HandleFunc("GET /{uuid}/files/{filename}", response.With(handler.updateFileGet))
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
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/updates?recursion=1 updates updates_get_recursion
//
//	Get the updates
//
//	Returns a list of updates (structs).
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
//	          items:
//	            $ref: "#/definitions/Update"
//	  "403":
//	    $ref: "#/responses/Forbidden"
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
				Channel:     update.Channel,
				Changelog:   update.Changelog,
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
//	  "403":
//	    $ref: "#/responses/Forbidden"
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
			Channel:     update.Channel,
			Changelog:   update.Changelog,
		},
		update,
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
//	  "403":
//	    $ref: "#/responses/Forbidden"
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
			Filename:  updateFile.Filename,
			URL:       updateFile.URL,
			Size:      updateFile.Size,
			Component: updateFile.Component,
			Type:      updateFile.Type,
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
//	  "403":
//	    $ref: "#/responses/Forbidden"
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

	return response.ReadCloserResponse(r, rc, filename, fileSize, nil)
}
