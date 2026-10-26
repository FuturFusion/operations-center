package api

import (
	"net/http"
	"path"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// internalMemberFilesHandler lets other cluster members read, copy and delete the files stored on this member.
type internalMemberFilesHandler struct {
	local memberfiles.Local

	// memberClient returns a client for the cluster member with the given address.
	memberClient func(address string) (client.OperationsCenterClient, error)
}

func registerInternalMemberFilesHandler(router Router, authorizer *authz.Authorizer, local memberfiles.Local, memberClient func(address string) (client.OperationsCenterClient, error)) {
	handler := &internalMemberFilesHandler{
		local:        local,
		memberClient: memberClient,
	}

	// Read a file of a store, or a directory as a tar archive.
	router.HandleFunc("GET /{store}/{name...}", response.With(handler.fileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	// Copy a directory of a store from the cluster member "source".
	router.HandleFunc("POST /{store}/{name...}", response.With(handler.pullPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
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

func (h *internalMemberFilesHandler) pullPost(r *http.Request) response.Response {
	source := r.URL.Query().Get("source")
	if source == "" {
		return response.SmartError(domain.NewValidationErrf(`Query parameter "source" is required`))
	}

	c, err := h.memberClient(source)
	if err != nil {
		return response.SmartError(err)
	}

	rc, _, err := c.GetMemberFile(r.Context(), r.PathValue("store"), r.PathValue("name"))
	if err != nil {
		return response.SmartError(err)
	}

	defer rc.Close()

	err = h.local.ExtractArchive(rc, r.PathValue("store"), r.PathValue("name"))
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (h *internalMemberFilesHandler) fileDelete(r *http.Request) response.Response {
	err := h.local.Delete(r.PathValue("store"), r.PathValue("name"))
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
