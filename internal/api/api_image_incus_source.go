package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type imageSourceHandler struct {
	service image.IncusImageSourceService
}

func registerIncusImageSourcesAndImagesHandler(router Router, simplestreamsRouter Router, authorizer *authz.Authorizer, sourceService image.IncusImageSourceService, imageService image.ImageIncusService) {
	handler := &imageSourceHandler{
		service: sourceService,
	}

	imageHandler := &imageIncusHandler{
		service:       imageService,
		sourceService: sourceService,
	}

	// public simplestreams routes, available without authentication
	simplestreamsRouter.HandleFunc("GET /streams/v1/index.json", response.With(imageHandler.simplestreamsPublicIndexGet))
	simplestreamsRouter.HandleFunc("GET /streams/v1/images.json", response.With(imageHandler.simplestreamsPublicImagesGet))
	// Allow download of incus image files without authentication
	simplestreamsRouter.HandleFunc("GET /1.0/images/incus/{source}/images/{name}/{version}/{filename}", response.With(imageHandler.incusImageVersionFileGet))

	router.HandleFunc("GET /{$}", response.With(handler.imageSourcesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.imageSourcesPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{source}", response.With(handler.imageSourceGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{source}", response.With(handler.imageSourcePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{source}", response.With(handler.imageSourceDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("POST /{source}/:refresh", response.With(handler.imageSourceRefreshPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))

	router.HandleFunc("GET /{source}/images", response.With(imageHandler.incusImagesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{source}/images", response.With(imageHandler.incusImageVersionPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /{source}/images/{name}", response.With(imageHandler.incusImageGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{source}/images/{name}", response.With(imageHandler.incusImagePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{source}/images/{name}", response.With(imageHandler.incusImageDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("DELETE /{source}/images/{name}/{version}", response.With(imageHandler.incusImageVersionDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("GET /{source}/images/{name}/{version}/{filename}", response.With(imageHandler.incusImageVersionFileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
}

// swagger:operation GET /1.0/images/incus image_sources image_sources_get
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
//	    $ref: "#/responses/URLsResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/images/incus?recursion=1 image_sources image_sources_get_recursion
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
//	    $ref: "#/responses/ImageSourcesResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourcesGet(r *http.Request) response.Response {
	if isRecursionRequest(r) {
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
		result = append(result, fmt.Sprintf("/%s/images/incus/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/images/incus image_sources image_sources_post
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
//	  "201":
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

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/images/incus/"+newImageSource.Name)
}

// swagger:operation GET /1.0/images/incus/{source} image_sources image_source_get
//
//	Get the image source
//
//	Gets a specific image source.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the source
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/ImageSourceResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourceGet(r *http.Request) response.Response {
	name := r.PathValue("source")

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

// swagger:operation PUT /1.0/images/incus/{source} image_sources image_source_put
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
//	  - in: path
//	    name: source
//	    description: Name of the source
//	    type: string
//	    required: true
//	  - in: body
//	    name: image_source
//	    description: Image source
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ImageSourcePut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourcePut(r *http.Request) response.Response {
	name := r.PathValue("source")

	var imageSource api.ImageSourcePut

	err := json.NewDecoder(r.Body).Decode(&imageSource)
	if err != nil {
		return response.BadRequest(err)
	}

	err = transaction.Do(r.Context(), func(ctx context.Context) error {
		currentImageSource, err := i.service.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get image source %q: %w", name, err)
		}

		// Validate ETag
		err = response.EtagCheck(r, currentImageSource)
		if err != nil {
			return err
		}

		currentImageSource.URL = imageSource.URL
		currentImageSource.FilterExpression = imageSource.FilterExpression

		err = i.service.Update(ctx, *currentImageSource)
		if err != nil {
			return fmt.Errorf("Failed updating image source %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(true, nil)
}

// swagger:operation DELETE /1.0/images/incus/{source} image_sources image_source_delete
//
//	Delete the image source
//
//	Removes the image source.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the source
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourceDelete(r *http.Request) response.Response {
	name := r.PathValue("source")

	err := i.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/images/incus/{source}/:refresh image_sources image_source_refresh_post
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
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the source
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageSourceHandler) imageSourceRefreshPost(r *http.Request) response.Response {
	name := r.PathValue("source")

	err := i.service.RefreshByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
