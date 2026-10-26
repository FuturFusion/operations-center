package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// internalImageFilesHandler lets other cluster members manage the image files stored on this member.
type internalImageFilesHandler struct {
	service       image.ImageIncusService
	sourceService image.IncusImageSourceService
}

func registerInternalImageFilesHandler(router Router, authorizer *authz.Authorizer, service image.ImageIncusService, sourceService image.IncusImageSourceService) {
	handler := &internalImageFilesHandler{
		service:       service,
		sourceService: sourceService,
	}

	// Download any missing files of an image from an image source and delete the files it no longer needs, in the background.
	router.HandleFunc("POST /{name}/:refresh", response.With(handler.syncPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
}

func (h *internalImageFilesHandler) syncPost(r *http.Request) response.Response {
	sourceName := r.URL.Query().Get("source")
	if sourceName == "" {
		return response.SmartError(domain.NewValidationErrf(`Query parameter "source" is required`))
	}

	var img image.IncusImage
	err := json.NewDecoder(r.Body).Decode(&img)
	if err != nil {
		return response.BadRequest(err)
	}

	source, err := h.sourceService.GetByName(r.Context(), sourceName)
	if err != nil {
		return response.SmartError(err)
	}

	// The synchronization outlives the request, the next refresh of the leader picks up any failure.
	ctx := context.WithoutCancel(r.Context())
	go func() {
		err := h.service.SyncLocalImage(ctx, *source, img)
		if err != nil {
			slog.WarnContext(ctx, "Failed to synchronize the files of an image", slog.String("image", img.Name), logger.Err(err))
		}
	}()

	return response.EmptySyncResponse
}
