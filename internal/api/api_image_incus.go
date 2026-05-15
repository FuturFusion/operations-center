package api

import (
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type imageIncusHandler struct {
	service image.ImageIncusService
}

func registerImageIncusHandler(router Router, authorizer *authz.Authorizer, service image.ImageIncusService) {
	handler := &imageIncusHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.incusImagesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("GET /{name}", response.With(handler.incusImageGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("DELETE /{name}", response.With(handler.incusImageDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}/{version}", response.With(handler.incusImageVersionPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}/{version}", response.With(handler.incusImageVersionDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("GET /{name}/{version}/{file}", response.With(handler.incusImageVersionFileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
}

// swagger:operation GET /1.0/image/incus incus_images incus_images_get
//
//	Get the list of incus images
//
//	Returns a list of incus images (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API incus images
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
//	          description: List of incus images
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/image/incus/almalinux:10:amd64:default",
//	              "/1.0/image/incus/almalinux:10:amd64:cloud"
//	            ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/image/incus?recursion=1 incus incus_get_recursion
//
//	Get the list of incus images
//
//	Returns a list of incus images (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API incus images
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
//	          description: List of incus
//	          items:
//	            $ref: "#/definitions/IncusImage"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImagesGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		incusImages, err := i.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.IncusImage, 0, len(incusImages))
		for _, incusImage := range incusImages {
			result = append(result, api.IncusImage{
				Name:            incusImage.Name,
				OperatingSystem: incusImage.OperatingSystem,
				Release:         incusImage.Release,
				Architecture:    incusImage.Architecture,
				Variant:         incusImage.Variant,
				Versions:        incusImage.Versions,
				LastUpdated:     incusImage.LastUpdated,

				IncusImagePut: api.IncusImagePut{
					Description: incusImage.Description,
				},
			})
		}

		return response.SyncResponse(true, result)
	}

	incusImageNames, err := i.service.GetAllNames(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(incusImageNames))
	for _, name := range incusImageNames {
		result = append(result, fmt.Sprintf("/%s/image/incus/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/image/incus/{name} incus_image incus_image_get
//
//	Get the incus image
//
//	Gets a specific incus image.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Incus image
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
//	          $ref: "#/definitions/IncusImage"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImageGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	incusImage, err := i.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.IncusImage{
			Name:            incusImage.Name,
			OperatingSystem: incusImage.OperatingSystem,
			Release:         incusImage.Release,
			Architecture:    incusImage.Architecture,
			Variant:         incusImage.Variant,
			Versions:        incusImage.Versions,
			LastUpdated:     incusImage.LastUpdated,

			IncusImagePut: api.IncusImagePut{
				Description: incusImage.Description,
			},
		},
		incusImage,
	)
}

// swagger:operation DELETE /1.0/image/incus/{name} incus_image incus_image_delete
//
//	Delete the incus image
//
//	Removes the incus image.
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
func (i *imageIncusHandler) incusImageDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := i.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/image/incus/{name}/{version} incus_image incus_images_post
//
//	Add an incus image version
//
//	Add a new incus image version. Also creates the incus image, if it does not
//	exist.
//
//	---
//	consumes:
//	  - multipart/form-data
//	produces:
//	  - application/json
//	requestBody:
//	  content:
//	    multipart/form-data:
//	      schema:
//	        type: object
//	        properties:
//	          filename:
//	            type: array
//	            items:
//	              type: string
//	              format: binary
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImageVersionPost(r *http.Request) response.Response {
	name := r.PathValue("name")
	version := r.PathValue("version")

	mediatype, params, err := mime.ParseMediaType(r.Header.Get("Content-type"))
	if err != nil {
		return response.BadRequest(fmt.Errorf("Failed to process Content-type header: %w", err))
	}

	if mediatype != "multipart/form-data" {
		return response.BadRequest(fmt.Errorf(`Content-type is not "multipart/form-data"`))
	}

	boundary, ok := params["boundary"]
	if !ok {
		return response.BadRequest(fmt.Errorf(`Content-type header misses boundary parameter`))
	}

	err = i.service.AddVersion(r.Context(), name, version, multipart.NewReader(r.Body, boundary))
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/image/incus/"+name)
}

// swagger:operation DELETE /1.0/image/incus/{name}/{version} incus_image incus_image_version_delete
//
//	Delete the incus image version
//
//	Removes the incus image version.
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
func (i *imageIncusHandler) incusImageVersionDelete(r *http.Request) response.Response {
	name := r.PathValue("name")
	version := r.PathValue("version")

	err := i.service.DeleteVersionByName(r.Context(), name, version)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (i *imageIncusHandler) incusImageVersionFileGet(r *http.Request) response.Response {
	// Download a file from a given incus image version
	return response.NotImplemented(fmt.Errorf("not implemented"))
}
