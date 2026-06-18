package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type imageSourceHandler struct {
	service image.IncusImageSourceService
}

func registerImageSourceHandler(router Router, authorizer *authz.Authorizer, service image.IncusImageSourceService) {
	handler := &imageSourceHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.imageSourcesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.imageSourcesPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{name}", response.With(handler.imageSourceGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.imageSourcePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.imageSourceDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{name}/:refresh", response.With(handler.imageSourceRefreshPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

// swagger:operation GET /1.0/images/incus/sources image_sources image_sources_get
//
//	Get the list of image sources
//
//	Returns a list of image sources (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API image sources
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
//	          description: List of image sources
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/images/incus/sources/linuxcontainer.org",
//	              "/1.0/images/incus/sources/images.org"
//	            ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/images/incus/sources?recursion=1 image_sources image_sources_get_recursion
//
//	Get the list of image sources
//
//	Returns a list of image sources (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API image sources
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
//	          description: List of image sources
//	          items:
//	            $ref: "#/definitions/ImageSource"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourcesGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		imageSources, err := i.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.ImageSource, 0, len(imageSources))
		for _, imageSource := range imageSources {
			result = append(result, api.ImageSource{
				LastUpdated: imageSource.LastUpdated,

				ImageSourcePost: api.ImageSourcePost{
					Name: imageSource.Name,

					ImageSourcePut: api.ImageSourcePut{
						URL:              imageSource.URL,
						FilterExpression: imageSource.FilterExpression,
					},
				},
			})
		}

		return response.SyncResponse(true, result)
	}

	imageSourceNames, err := i.service.GetAllNames(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(imageSourceNames))
	for _, name := range imageSourceNames {
		result = append(result, fmt.Sprintf("/%s/image/sources/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/images/incus/sources image_sources image_sources_post
//
//	Add an image source
//
//	Creates a new image source.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: image_source
//	    description: Image source configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ImageSourcePost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourcesPost(r *http.Request) response.Response {
	var imageSource api.ImageSourcePost

	// Decode into the new image source.
	err := json.NewDecoder(r.Body).Decode(&imageSource)
	if err != nil {
		return response.BadRequest(err)
	}

	newImageSource, err := i.service.Create(r.Context(), image.IncusImageSource{
		Name:             imageSource.Name,
		URL:              imageSource.URL,
		FilterExpression: imageSource.FilterExpression,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating image source: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/image/sources/"+newImageSource.Name)
}

// swagger:operation GET /1.0/images/incus/sources/{name} image_source image_source_get
//
//	Get the image source
//
//	Gets a specific image source.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Image source
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
//	          $ref: "#/definitions/ImageSource"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourceGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	imageSource, err := i.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.ImageSource{
			LastUpdated: imageSource.LastUpdated,

			ImageSourcePost: api.ImageSourcePost{
				Name: imageSource.Name,

				ImageSourcePut: api.ImageSourcePut{
					URL:              imageSource.URL,
					FilterExpression: imageSource.FilterExpression,
				},
			},
		},
		imageSource,
	)
}

// swagger:operation PUT /1.0/images/incus/sources/{name} image_source image_source_put
//
//	Update the image source
//
//	Updates the image source.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: image_source
//	    description: Image source
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ImageSource"
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
func (i *imageSourceHandler) imageSourcePut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var imageSource api.ImageSourcePut

	err := json.NewDecoder(r.Body).Decode(&imageSource)
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

	currentImageSource, err := i.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get image source %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentImageSource)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	currentImageSource.URL = imageSource.URL
	currentImageSource.FilterExpression = imageSource.FilterExpression

	err = i.service.Update(ctx, *currentImageSource)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating image source %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/image/sources/"+name)
}

// swagger:operation DELETE /1.0/images/incus/sources/{name} image_source image_source_delete
//
//	Delete the image source
//
//	Removes the image source.
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
func (i *imageSourceHandler) imageSourceDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := i.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/images/incus/sources/{name}/:refresh image_source image_source_refresh_post
//
//	Refresh the image source
//
//	Refresh the image source by fetching the available images, which
//	pass the filter, from origin and update the state in the DB and in the
//	files repository.
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
func (i *imageSourceHandler) imageSourceRefreshPost(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := i.service.RefreshByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
