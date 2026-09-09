package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type secureBootMediaHandler struct {
	media provisioning.SecureBootMediaPort
}

func registerSecureBootMediaHandler(router Router, media provisioning.SecureBootMediaPort) {
	handler := &secureBootMediaHandler{media: media}

	router.HandleFunc("GET /{filename}", response.With(handler.secureBootMediaGet))
}

// swagger:operation GET /1.0/provisioning/secure-boot-media/{filename} secure_boot_media secure_boot_media_get
//
//	Get a secure boot enrollment media
//
//	Get the generated secure boot enrollment media, which enrolls the UEFI key
//	databases of a server.
//
//	The media is addressed by its content alone, so the endpoint requires no
//	authentication. A media, that has not been generated, is reported as
//	"404 Not Found".
//
//	The filename is the ID of the media with the extension of its image type,
//	e.g. ".iso" or ".raw".
//
//	The media is served with a Content-Length header and support for byte
//	ranges, so a BMC whose transfer got interrupted can resume it. HEAD is
//	answered with the headers alone.
//
//	---
//	produces:
//	  - application/octet-stream
//	parameters:
//	  - in: path
//	    name: filename
//	    description: |-
//	      ID of the generated media with the extension of its image type, e.g.
//	      "a1B2c3D4e5F6.iso".
//	    type: string
//	    required: true
//	  - in: header
//	    name: Range
//	    description: |-
//	      Byte range of the media to return, e.g. "bytes=1048576-".
//	    type: string
//	    required: false
//	responses:
//	  "200":
//	    $ref: "#/responses/SecureBootMediaResponse"
//	  "206":
//	    $ref: "#/responses/SecureBootMediaResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "404":
//	    $ref: "#/responses/NotFound"
//	  "416":
//	    $ref: "#/responses/BadRequest"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (s *secureBootMediaHandler) secureBootMediaGet(r *http.Request) response.Response {
	filename := r.PathValue("filename")

	extension := filepath.Ext(filename)

	imageType, ok := api.ImageTypeForFileExt(extension)
	if !ok {
		return response.BadRequest(fmt.Errorf("Secure boot enrollment media %q does not have the extension of an image type", filename))
	}

	image, err := s.media.Open(r.Context(), imageType, strings.TrimSuffix(filename, extension))
	if err != nil {
		return response.SmartError(err)
	}

	return response.ServeContentResponse(r, image.Content, image.Filename, image.ModTime, image.Size, nil)
}
