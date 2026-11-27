package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// memberRefreshWarnings records a failed refresh of the files stored on this member as a warning, which its next successful refresh removes.
type memberRefreshWarnings struct {
	emitter    warning.WarningEmitter
	memberName func(ctx context.Context) string
}

// report records err of the refresh of the files of entity, or removes the warning if the refresh succeeded.
func (w memberRefreshWarnings) report(ctx context.Context, entityType string, entity string, err error) {
	// The leader's own refresh uses another scope, so its success doesn't remove this warning.
	scope := api.WarningScope{Scope: "member_refresh", EntityType: entityType, Entity: entity}

	if err != nil {
		w.emitter.Emit(ctx, warning.NewWarningFromError(api.WarningTypeUpdateRefreshFailed, scope, err, "Refresh of the files of %s %q on cluster member %q failed", entityType, entity, w.memberName(ctx)))
		return
	}

	w.emitter.RemoveStale(ctx, scope, nil)
}

// internalUpdateFilesHandler lets other cluster members manage the update files stored on this member.
type internalUpdateFilesHandler struct {
	service  provisioning.UpdateService
	warnings memberRefreshWarnings
}

func registerInternalUpdateFilesHandler(router Router, authorizer *authz.Authorizer, service provisioning.UpdateService, warnings memberRefreshWarnings) {
	handler := &internalUpdateFilesHandler{
		service:  service,
		warnings: warnings,
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
		h.warnings.report(ctx, "update", id.String(), h.service.RefreshLocalFiles(ctx, id))
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
