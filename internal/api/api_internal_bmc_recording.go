package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

type internalBMCRecordingHandler struct {
	service provisioning.ServerService
}

func registerInternalBMCRecordingHandler(router Router, authorizer *authz.Authorizer, service provisioning.ServerService) {
	handler := &internalBMCRecordingHandler{
		service: service,
	}

	canAdmin := assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)

	router.HandleFunc("GET /bmc-recordings/{server}", response.With(handler.recordingsGet, canAdmin))
	router.HandleFunc("GET /bmc-recordings/{server}/{id}", response.With(handler.recordingGet, canAdmin))
	router.HandleFunc("DELETE /bmc-recordings/{server}/{id}", response.With(handler.recordingDelete, canAdmin))
	router.HandleFunc("GET /bmc-recordings/{server}/{id}/{filename}", response.With(handler.recordingFileGet, canAdmin))
}

func (h *internalBMCRecordingHandler) recordingsGet(r *http.Request) response.Response {
	name := r.PathValue("server")

	manifests, err := h.service.GetBMCRecordings(r.Context(), name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get BMC recordings of server %q: %w", name, err))
	}

	return response.SyncResponse(true, manifests)
}

func (h *internalBMCRecordingHandler) recordingGet(r *http.Request) response.Response {
	name := r.PathValue("server")
	id := r.PathValue("id")

	archive := r.FormValue("archive")
	if archive != "" {
		if archive != "zip" {
			return response.BadRequest(fmt.Errorf("Archive type %q not supported", archive))
		}

		anonymize := false

		anonymizeValue := r.FormValue("anonymize")
		if anonymizeValue != "" {
			var err error

			anonymize, err = strconv.ParseBool(anonymizeValue)
			if err != nil {
				return response.BadRequest(fmt.Errorf("Invalid value %q for query parameter %q: %w", anonymizeValue, "anonymize", err))
			}
		}

		rc, size, err := h.service.GetBMCRecordingArchive(r.Context(), name, id, anonymize)
		if err != nil {
			return response.SmartError(fmt.Errorf("Failed to get archive of BMC recording %q of server %q: %w", id, name, err))
		}

		headers := map[string]string{
			"Content-Type": "application/zip",
		}

		// The filename matches the server name in the anonymized manifest.
		filenameServer := name
		if anonymize {
			filenameServer = provisioning.BMCRecordingServerPlaceholder
		}

		// The zip is compressed already.
		return response.ReadCloserResponse(r, rc, false, fmt.Sprintf("%s-%s.zip", filenameServer, id), size, headers)
	}

	manifest, err := h.service.GetBMCRecording(r.Context(), name, id)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get BMC recording %q of server %q: %w", id, name, err))
	}

	return response.SyncResponse(true, manifest)
}

func (h *internalBMCRecordingHandler) recordingFileGet(r *http.Request) response.Response {
	name := r.PathValue("server")
	id := r.PathValue("id")
	filename := r.PathValue("filename")

	rc, size, err := h.service.GetBMCRecordingFile(r.Context(), name, id, filename)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get file %q of BMC recording %q of server %q: %w", filename, id, name, err))
	}

	// ReadCloserResponse replaces a plain "application/json" as its default.
	contentType := "application/json; charset=utf-8"
	if strings.HasSuffix(filename, ".jsonl") {
		contentType = "application/x-ndjson"
	}

	headers := map[string]string{
		"Content-Type": contentType,
	}

	return response.ReadCloserResponse(r, rc, true, filename, int(size), headers)
}

func (h *internalBMCRecordingHandler) recordingDelete(r *http.Request) response.Response {
	name := r.PathValue("server")
	id := r.PathValue("id")

	err := h.service.DeleteBMCRecording(r.Context(), name, id)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to delete BMC recording %q of server %q: %w", id, name, err))
	}

	return response.EmptySyncResponse
}
