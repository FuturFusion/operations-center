package api

import (
	"net/http"
	"path"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// internalMemberFilesHandler lets other cluster members read and delete the files stored on this member.
type internalMemberFilesHandler struct {
	local memberfiles.Local
}

func registerInternalMemberFilesHandler(router Router, authorizer *authz.Authorizer, local memberfiles.Local) {
	handler := &internalMemberFilesHandler{
		local: local,
	}

	// Read a file of a store, or a directory as a tar archive.
	router.HandleFunc("GET /{store}/{name...}", response.With(handler.fileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	// Delete a file or directory of a store.
	router.HandleFunc("DELETE /{store}/{name...}", response.With(handler.fileDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
	// Delete all files of a store.
	router.HandleFunc("DELETE /{store}", response.With(handler.fileDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
}

func (h *internalMemberFilesHandler) fileGet(r *http.Request) response.Response {
	store := r.PathValue("store")
	name := r.PathValue("name")

	info, err := h.local.Stat(store, name)
	if err != nil {
		return response.SmartError(err)
	}

	if info.IsDir() {
		return response.ManualResponse(func(w http.ResponseWriter) error {
			w.Header().Set("Content-Type", "application/x-tar")
			return h.local.WriteArchive(w, store, name)
		})
	}

	f, err := h.local.Open(store, name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.ReadCloserResponse(r, f, false, path.Base(name), int(info.Size()), nil)
}

func (h *internalMemberFilesHandler) fileDelete(r *http.Request) response.Response {
	err := h.local.Delete(r.PathValue("store"), r.PathValue("name"))
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
