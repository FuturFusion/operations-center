package api

import (
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/simplestreams"
)

type imageIncusHandler struct {
	service       image.ImageIncusService
	sourceService image.IncusImageSourceService
}

// swagger:operation GET /incus-images/streams/v1/index.json simplestreams simplestreams_index_get
//
//	Get incus images simplestreams index
//
//	Returns the list of publicly available incus images in simplestreams v1
//
// format, served at streams/v1/index.json.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Simplestreams API for incus images index
//	    schema:
//	      $ref: "#/definitions/SimplestreamsStream"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) simplestreamsPublicIndexGet(r *http.Request) response.Response {
	incusImages, err := i.service.GetAll(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	index := simplestreams.StreamIndex{
		DataType: "image-downloads",
		Path:     "streams/v1/images.json",
		Format:   "products:1.0",
		Products: make([]string, 0, len(incusImages)),
	}

	for _, incusImage := range incusImages {
		index.Products = append(index.Products, simplestreamsProductName(incusImage))
	}

	stream := simplestreams.Stream{
		Index: map[string]simplestreams.StreamIndex{
			"images": index,
		},
		Format: "index:1.0",
	}

	return response.ManualResponse(func(w http.ResponseWriter) error {
		w.WriteHeader(http.StatusOK)

		return json.NewEncoder(w).Encode(stream)
	})
}

// swagger:operation GET /incus-images/streams/v1/images.json simplestreams simplestreams_images_get
//
//	Get incus images simplestreams images
//
//	Returns the details for the publicly available incus images in simplestreams
//	v1 format, served at streams/v1/images.json.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Simplestreams API for incus images details
//	    schema:
//	      $ref: "#/definitions/SimplestreamsProducts"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) simplestreamsPublicImagesGet(r *http.Request) response.Response {
	incusImages, err := i.service.GetAll(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	products := simplestreams.Products{
		ContentID: "images",
		DataType:  "image-downloads",
		Format:    "products:1.0",
		Products:  make(map[string]simplestreams.Product, len(incusImages)),
	}

	// Images of different sources can have the same alias. Incus picks one of
	// them without a warning. So only the preferred image keeps the alias.
	// Local images win, then the other sources in the order of their ID.
	// GetAll orders the images by source ID, so only local images move to the front.
	sortLocalIncusImagesFirst(incusImages)

	type aliasKey struct {
		alias        string
		architecture string
	}

	takenAliases := map[aliasKey]bool{}

	for _, incusImage := range incusImages {
		// Incus cannot launch an image without versions. It must not take the
		// aliases of the images of other sources.
		if len(incusImage.Versions) == 0 {
			continue
		}

		aliases := make([]string, 0, len(incusImage.Aliases))
		for _, alias := range incusImage.Aliases {
			key := aliasKey{alias: alias, architecture: incusImage.Architecture}
			if takenAliases[key] {
				continue
			}

			takenAliases[key] = true
			aliases = append(aliases, alias)
		}

		product := simplestreams.Product{
			Aliases:         strings.Join(aliases, ","),
			Architecture:    incusImage.Architecture,
			OperatingSystem: incusImage.OperatingSystem,
			Release:         incusImage.Release,
			Versions:        incusImage.Versions,
			Variant:         incusImage.Variant,
		}

		for version := range product.Versions {
			for filename, fileDetails := range product.Versions[version].Items {
				fileDetails.Path = filepath.Join("/1.0/images/incus", incusImage.Source, "images", incusImage.Name, version, filename)
				product.Versions[version].Items[filename] = fileDetails
			}
		}

		products.Products[simplestreamsProductName(incusImage)] = product
	}

	return response.ManualResponse(func(w http.ResponseWriter) error {
		w.WriteHeader(http.StatusOK)

		return json.NewEncoder(w).Encode(products)
	})
}

// swagger:operation GET /1.0/images/incus/{source}/images incus_images incus_images_get
//
//	Get the list of incus images
//
//	Returns a list of incus images (URLs) of the image source.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/URLsResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/images/incus/{source}/images?recursion=1 incus_images incus_images_get_recursion
//
//	Get the list of incus images
//
//	Returns a list of incus images (structs) of the image source.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/IncusImagesResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImagesGet(r *http.Request) response.Response {
	source := r.PathValue("source")

	// An unknown source is not found, not an empty list.
	_, err := i.sourceService.GetByName(r.Context(), source)
	if err != nil {
		return response.SmartError(err)
	}

	incusImages, err := i.service.GetAllBySource(r.Context(), source)
	if err != nil {
		return response.SmartError(err)
	}

	if isRecursionRequest(r) {
		result := make([]api.IncusImage, 0, len(incusImages))
		for _, incusImage := range incusImages {
			result = append(result, api.IncusImage{
				Source:          incusImage.Source,
				Name:            incusImage.Name,
				OperatingSystem: incusImage.OperatingSystem,
				Release:         incusImage.Release,
				Architecture:    incusImage.Architecture,
				Variant:         incusImage.Variant,
				Versions:        incusImage.Versions,
				LastUpdated:     incusImage.LastUpdated,

				IncusImagePut: api.IncusImagePut{
					Aliases:     incusImage.Aliases,
					Description: incusImage.Description,
				},
			})
		}

		return response.SyncResponse(true, result)
	}

	result := make([]string, 0, len(incusImages))
	for _, incusImage := range incusImages {
		result = append(result, fmt.Sprintf("/%s/images/incus/%s/images/%s", api.APIVersion, incusImage.Source, incusImage.Name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/images/incus/{source}/images/{name} incus_images incus_image_get
//
//	Get the incus image
//
//	Gets a specific incus image.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the incus image
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    $ref: "#/responses/IncusImageResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImageGet(r *http.Request) response.Response {
	source := r.PathValue("source")
	name := r.PathValue("name")

	incusImage, err := i.service.GetByName(r.Context(), source, name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.IncusImage{
			Source:          incusImage.Source,
			Name:            incusImage.Name,
			OperatingSystem: incusImage.OperatingSystem,
			Release:         incusImage.Release,
			Architecture:    incusImage.Architecture,
			Variant:         incusImage.Variant,
			Versions:        incusImage.Versions,
			LastUpdated:     incusImage.LastUpdated,

			IncusImagePut: api.IncusImagePut{
				Aliases:     incusImage.Aliases,
				Description: incusImage.Description,
			},
		},
		incusImage,
	)
}

// swagger:operation PUT /1.0/images/incus/{source}/images/{name} incus_images incus_image_put
//
//	Update the incus image
//
//	Updates the incus image.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the incus image
//	    type: string
//	    required: true
//	  - in: body
//	    name: incus_image
//	    description: Incus image
//	    required: true
//	    schema:
//	      $ref: "#/definitions/IncusImagePut"
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
func (i *imageIncusHandler) incusImagePut(r *http.Request) response.Response {
	source := r.PathValue("source")
	name := r.PathValue("name")

	var incusImage api.IncusImagePut

	err := json.NewDecoder(r.Body).Decode(&incusImage)
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

	currentIncusImage, err := i.service.GetByName(ctx, source, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get incus image %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentIncusImage)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	currentIncusImage.Aliases = incusImage.Aliases
	currentIncusImage.Description = incusImage.Description

	err = i.service.Update(ctx, *currentIncusImage)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating incus image %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponse(true, nil)
}

// swagger:operation DELETE /1.0/images/incus/{source}/images/{name} incus_images incus_image_delete
//
//	Delete the incus image
//
//	Removes the incus image.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the incus image
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
func (i *imageIncusHandler) incusImageDelete(r *http.Request) response.Response {
	source := r.PathValue("source")
	name := r.PathValue("name")

	err := i.service.DeleteByName(r.Context(), source, name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/images/incus/{source}/images incus_images incus_images_post
//
//	Add an incus image version
//
//	Add a new incus image version. Also creates the incus image, if it does not
//	exist. Images can only be uploaded to the image source "local".
//
//	---
//	consumes:
//	  - multipart/form-data
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source, only "local" is permitted
//	    type: string
//	    required: true
//	  - in: formData
//	    name: request_json
//	    description: >
//	      The fields of the request, as a JSON object of the IncusImagePost
//	      definition. In order to allow stream processing of the request, the
//	      order of the fields matters: request_json has to be sent first. If it
//	      is omitted, the first part is required to be the metadata tarball of
//	      the image, which is recognized by its content and is stored as
//	      incus.tar.xz, independent of the name it is uploaded with.
//	    type: string
//	  - in: formData
//	    name: fileNN
//	    description: >
//	      A single request may carry multiple files. Except for request_json,
//	      the form field names are irrelevant, the CLI prefixes them with
//	      "file", followed by an incrementing number, e.g. file00, file01. The
//	      type of every image file is detected from its content, the file name
//	      is ignored.
//	    type: file
//	responses:
//	  "201":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImageVersionPost(r *http.Request) response.Response {
	source := r.PathValue("source")
	if source != api.IncusImageSourceLocal {
		return response.SmartError(domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Images can only be uploaded to the image source %q", api.IncusImageSourceLocal).
			WithDetail("source", source))
	}

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

	name, err := i.service.AddVersion(r.Context(), multipart.NewReader(r.Body, boundary))
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/images/incus/"+api.IncusImageSourceLocal+"/images/"+name)
}

// swagger:operation DELETE /1.0/images/incus/{source}/images/{name}/{version} incus_images incus_image_version_delete
//
//	Delete the incus image version
//
//	Removes the incus image version.
//
//	---
//	produces:
//	  - application/json
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: version
//	    description: Version of the image
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
func (i *imageIncusHandler) incusImageVersionDelete(r *http.Request) response.Response {
	source := r.PathValue("source")
	name := r.PathValue("name")
	version := r.PathValue("version")

	err := i.service.DeleteVersionByName(r.Context(), source, name, version)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/images/incus/{source}/images/{name}/{version}/{filename} incus_images incus_image_version_file_get
//
//	Get a specific file from an incus image version
//
//	Gets a specific file from an incus image version.
//
//	---
//	produces:
//	  - "application/octet-stream"
//	parameters:
//	  - in: path
//	    name: source
//	    description: Name of the image source of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: name
//	    description: Name of the incus image
//	    type: string
//	    required: true
//	  - in: path
//	    name: version
//	    description: Version of the image
//	    type: string
//	    required: true
//	  - in: path
//	    name: filename
//	    description: Name of the file
//	    type: string
//	    required: true
//	responses:
//	  "200":
//	    description: File content
//	    schema:
//	      type: file
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (i *imageIncusHandler) incusImageVersionFileGet(r *http.Request) response.Response {
	source := r.PathValue("source")
	name := r.PathValue("name")
	version := r.PathValue("version")
	filename := r.PathValue("filename")

	rc, size, err := i.service.GetVersionFileByName(r.Context(), source, name, version, filename)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get file %q for incus image %q, version %q: %w", filename, name, version, err))
	}

	headers := map[string]string{
		"Content-Type": "application/octet-stream",
	}

	return response.ReadCloserResponse(r, rc, false, filename, int(size), headers)
}

func sortLocalIncusImagesFirst(images image.IncusImages) {
	slices.SortStableFunc(images, func(a image.IncusImage, b image.IncusImage) int {
		aLocal := a.Source == api.IncusImageSourceLocal
		bLocal := b.Source == api.IncusImageSourceLocal
		if aLocal == bLocal {
			return 0
		}

		if aLocal {
			return -1
		}

		return 1
	})
}

func simplestreamsProductName(incusImage image.IncusImage) string {
	// Images of different sources can have the same name.
	return incusImage.Source + ":" + incusImage.Name
}
