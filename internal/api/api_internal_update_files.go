package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// internalUpdateFilesHandler lets other cluster members manage the update files stored on this member.
type internalUpdateFilesHandler struct {
	service provisioning.UpdateService
}

func registerInternalUpdateFilesHandler(router Router, authorizer *authz.Authorizer, service provisioning.UpdateService) {
	handler := &internalUpdateFilesHandler{
		service: service,
	}

	// Download any missing files of an update and delete the files it no longer needs, in the background.
	router.HandleFunc("POST /{uuid}/:refresh", response.With(handler.refreshPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	// Return the updates whose files are missing or incomplete.
	router.HandleFunc("POST /:check", response.With(handler.checkPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
}

func (h *internalUpdateFilesHandler) refreshPost(r *http.Request) response.Response {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		return response.BadRequest(err)
	}

	// The refresh outlives the request, the next refresh of the leader picks up any failure.
	ctx := context.WithoutCancel(r.Context())
	go func() {
		err := h.service.RefreshLocalFiles(ctx, id)
		if err != nil {
			slog.WarnContext(ctx, "Failed to refresh the files of an update", slog.String("update", id.String()), logger.Err(err))
		}
	}()

	return response.EmptySyncResponse
}

func (h *internalUpdateFilesHandler) checkPost(r *http.Request) response.Response {
	var ids []uuid.UUID
	err := json.NewDecoder(r.Body).Decode(&ids)
	if err != nil {
		return response.BadRequest(err)
	}

	updates := make(provisioning.Updates, 0, len(ids))
	for _, id := range ids {
		update, err := h.service.GetByUUID(r.Context(), id)
		if err != nil {
			return response.SmartError(err)
		}

		updates = append(updates, *update)
	}

	incomplete, err := h.service.CheckLocalFiles(r.Context(), updates)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(true, incomplete)
}
