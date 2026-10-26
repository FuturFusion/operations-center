package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/simplestreams"
)

type imageIncusHandler struct {
	service image.ImageIncusService
	forward forwarder
}

func registerImageIncusHandler(router Router, simplestreamsRouter Router, authorizer *authz.Authorizer, service image.ImageIncusService, forward forwarder) {
	handler := &imageIncusHandler{
		service: service,
		forward: forward,
	}

	// public simplestreams routes, available without authentication
	simplestreamsRouter.HandleFunc("GET /streams/v1/index.json", response.With(handler.simplestreamsPublicIndexGet))
	simplestreamsRouter.HandleFunc("GET /streams/v1/images.json", response.With(handler.simplestreamsPublicImagesGet))
	// Allow download of incus image files without authentication
	simplestreamsRouter.HandleFunc("GET /1.0/images/incus/{name}/{version}/{filename}", response.With(handler.incusImageVersionFileGet))

	router.HandleFunc("GET /{$}", response.With(handler.incusImagesGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.incusImageVersionPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("GET /{name}", response.With(handler.incusImageGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.incusImagePut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.incusImageDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("DELETE /{name}/{version}", response.With(handler.incusImageVersionDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	router.HandleFunc("GET /{name}/{version}/{filename}", response.With(handler.incusImageVersionFileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
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
		index.Products = append(index.Products, incusImage.Name)
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

	for _, incusImage := range incusImages {
		product := simplestreams.Product{
			Aliases:         strings.Join(incusImage.Aliases, ","),
			Architecture:    incusImage.Architecture,
			OperatingSystem: incusImage.OperatingSystem,
			Release:         incusImage.Release,
			Versions:        incusImage.Versions,
			Variant:         incusImage.Variant,
		}

		for version := range product.Versions {
			for filename, fileDetails := range product.Versions[version].Items {
				fileDetails.Path = filepath.Join("/1.0/images/incus", incusImage.Name, version, filename)
				product.Versions[version].Items[filename] = fileDetails
			}
		}

		products.Products[incusImage.Name] = product
	}

	return response.ManualResponse(func(w http.ResponseWriter) error {
		w.WriteHeader(http.StatusOK)

		return json.NewEncoder(w).Encode(products)
	})
}

// swagger:operation GET /1.0/images/incus incus_images incus_images_get
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
//	    $ref: "#/responses/URLsResponse"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/images/incus?recursion=1 incus_images incus_images_get_recursion
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
//	    $ref: "#/responses/IncusImagesResponse"
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
					Aliases:     incusImage.Aliases,
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
		result = append(result, fmt.Sprintf("/%s/images/incus/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation GET /1.0/images/incus/{name} incus_images incus_image_get
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
				Aliases:     incusImage.Aliases,
				Description: incusImage.Description,
			},
		},
		incusImage,
	)
}

// swagger:operation PUT /1.0/images/incus/{name} incus_images incus_image_put
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
	name := r.PathValue("name")

	var incusImage api.IncusImagePut

	err := json.NewDecoder(r.Body).Decode(&incusImage)
	if err != nil {
		return response.BadRequest(err)
	}

	err = transaction.Do(r.Context(), func(ctx context.Context) error {
		currentIncusImage, err := i.service.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get incus image %q: %w", name, err)
		}

		// Validate ETag
		err = response.EtagCheck(r, currentIncusImage)
		if err != nil {
			return err
		}

		currentIncusImage.Aliases = incusImage.Aliases
		currentIncusImage.Description = incusImage.Description

		err = i.service.Update(ctx, *currentIncusImage)
		if err != nil {
			return fmt.Errorf("Failed updating incus image %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(true, nil)
}

// swagger:operation DELETE /1.0/images/incus/{name} incus_images incus_image_delete
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
	name := r.PathValue("name")

	resp := i.toFileTarget(r, name)
	if resp != nil {
		return resp
	}

	err := i.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation POST /1.0/images/incus incus_images incus_images_post
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
//	parameters:
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

	// The first part names the image, a version of an existing image is added by its file target.
	recorded := &cappedBuffer{limit: uploadFirstPartLimit}
	imageName, err := i.service.UploadImageName(r.Context(), multipart.NewReader(io.TeeReader(r.Body, recorded), boundary))
	if err != nil {
		return response.SmartError(err)
	}

	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(recorded.Bytes()), r.Body), r.Body}

	resp := i.toFileTarget(r, imageName)
	if resp != nil {
		return resp
	}

	name, err := i.service.AddVersion(r.Context(), multipart.NewReader(r.Body, boundary))
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/images/incus/"+name)
}

// swagger:operation DELETE /1.0/images/incus/{name}/{version} incus_images incus_image_version_delete
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
	name := r.PathValue("name")
	version := r.PathValue("version")

	resp := i.toFileTarget(r, name)
	if resp != nil {
		return resp
	}

	err := i.service.DeleteVersionByName(r.Context(), name, version)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

// swagger:operation GET /1.0/images/incus/{name}/{version}/{filename} incus_images incus_image_version_file_get
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
	name := r.PathValue("name")
	version := r.PathValue("version")
	filename := r.PathValue("filename")

	resp := i.toFileTarget(r, name)
	if resp != nil {
		return resp
	}

	rc, size, err := i.service.GetVersionFileByName(r.Context(), name, version, filename)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get file %q for incus image %q, version %q: %w", filename, name, version, err))
	}

	headers := map[string]string{
		"Content-Type": "application/octet-stream",
	}

	return response.ReadCloserResponse(r, rc, false, filename, int(size), headers)
}

// toFileTarget forwards the request to the associated cluster member, returning its response.
func (i *imageIncusHandler) toFileTarget(r *http.Request, name string) response.Response {
	img, err := i.service.GetByName(r.Context(), name)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}

	if err != nil {
		return response.SmartError(err)
	}

	return i.forward.toMember(r, img.MemberID)
}

// uploadFirstPartLimit limits the first part of an image upload, which is held in memory to learn the image name.
const uploadFirstPartLimit = 32 * 1024 * 1024

// cappedBuffer records up to limit bytes and fails once more are written.
type cappedBuffer struct {
	bytes.Buffer

	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.Len()+len(p) > c.limit {
		return 0, domain.NewValidationErrf("The first part of the image upload exceeds %d bytes", c.limit)
	}

	return c.Buffer.Write(p)
}
