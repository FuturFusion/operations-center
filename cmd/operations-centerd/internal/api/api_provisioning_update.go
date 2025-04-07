package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/provisioning"
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

	router.HandleFunc("GET /{$}", response.With(handler.updatesGet))
	router.HandleFunc("GET /{id}", response.With(handler.updateGet))
	router.HandleFunc("GET /{id}/files", response.With(handler.updateFilesGet))
	router.HandleFunc("GET /{id}/files/{filename}", response.With(handler.updateFileGet))
}

// swagger:operation GET /1.0/updates updates updates_get
//
//	Get the updates
//
//	Returns a list of updates (URLs).
//
//	---
//	produces:
//	  - application/json
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
//                    "/1.0/updates/b32d0079-c48b-4957-b1cb-bef54125c861",
//                    "/1.0/updates/464d229b-3069-4a82-bc59-b215a7c6ed1b"
//                  ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/updates?recursion=1 updates updates_get_recursion
//
//	Get the updates
//
//	Returns a list of updates (structs).
//
//	---
//	produces:
//	  - application/json
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

	if recursion == 1 {
		updates, err := u.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Update, 0, len(updates))
		for _, update := range updates {
			result = append(result, api.Update{
				ID:          update.ID,
				Components:  update.Components,
				Version:     update.Version,
				PublishedAt: update.PublishedAt,
				Severity:    update.Severity,
				Channel:     update.Channel,
			})
		}

		return response.SyncResponse(true, result)
	}

	updateIDs, err := u.service.GetAllIDs(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(updateIDs))
	for _, id := range updateIDs {
		result = append(result, fmt.Sprintf("/%s/updates/%s", api.APIVersion, id))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/updates/{id} updates update_get
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
	id := r.PathValue("id")

	update, err := u.service.GetByID(r.Context(), id)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Update{
			ID:          update.ID,
			Components:  update.Components,
			Version:     update.Version,
			PublishedAt: update.PublishedAt,
			Severity:    update.Severity,
			Channel:     update.Channel,
		},
		update,
	)
}

// swagger:operation GET /1.0/updates/{id}/files updates updates_files_get
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
	id := r.PathValue("id")

	updateFiles, err := u.service.GetUpdateAllFiles(r.Context(), id)
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]api.UpdateFile, 0, len(updateFiles))
	for _, updateFile := range updateFiles {
		result = append(result, api.UpdateFile{
			UpdateID: updateFile.UpdateID,
			Filename: updateFile.Filename,
			URL:      updateFile.URL,
			Size:     updateFile.Size,
		})
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/updates/{id}/files/{filename} update update_file_get
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
	id := r.PathValue("id")
	filename := r.PathValue("filename")

	rc, fileSize, err := u.service.GetUpdateFileByFilename(r.Context(), id, filename)
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, rc, filename, fileSize, nil)
}
