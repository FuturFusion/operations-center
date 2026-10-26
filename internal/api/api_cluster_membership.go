package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	clusterSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type membershipHandler struct {
	authorizer *authz.Authorizer
	service    clusterSvc.MembershipService

	// restart restarts the daemon, so it comes up clustered.
	restart func()

	// mover moves the files of a member to another one.
	mover memberfiles.Mover
}

func registerClusterMembershipHandler(
	router Router,
	authorizer *authz.Authorizer,
	service clusterSvc.MembershipService,
	restart func(),
	mover memberfiles.Mover,
) {
	handler := &membershipHandler{
		authorizer: authorizer,
		service:    service,
		restart:    restart,
		mover:      mover,
	}

	// Clustering:
	//
	// Enable clustering as the first member.
	router.HandleFunc("POST /:enable", response.With(handler.enablePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
	// Enable clustering by joining a cluster.
	router.HandleFunc("POST /:join", response.With(handler.joinPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
	// Get cluster members.
	router.HandleFunc("GET /members", response.With(handler.membersGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	// Remove a cluster member.
	router.HandleFunc("DELETE /members/{name}", response.With(handler.memberDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
	// Move the files of a cluster member to another one.
	router.HandleFunc("POST /members/{name}/:move-files", response.With(handler.memberMoveFilesPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))

	// Tokens:
	//
	// Issue a join token.
	router.HandleFunc("POST /tokens", response.With(handler.tokenPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
	// Get join tokens.
	router.HandleFunc("GET /tokens", response.With(handler.tokensGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
	// Get a join token for a candidate.
	router.HandleFunc("GET /tokens/{name}", response.With(handler.tokenGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
	// Delete a join token for a candidate.
	router.HandleFunc("DELETE /tokens/{name}", response.With(handler.tokenDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
}

func (m *membershipHandler) enablePost(r *http.Request) response.Response {
	var req api.ClusterBootstrapPost
	// Parse the request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	err = m.service.Bootstrap(r.Context(), req)
	if err != nil {
		return response.SmartError(err)
	}

	m.restart()

	return response.EmptySyncResponse
}

func (m *membershipHandler) joinPost(r *http.Request) response.Response {
	var req api.ClusterJoinPost
	// Parse the request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	err = m.service.Join(r.Context(), req)
	if errors.Is(err, clusterSvc.ErrJoinReset) {
		// Restart the daemon as standalone server.
		m.restart()
	}

	if err != nil {
		return response.BadRequest(err)
	}

	m.restart()

	return response.EmptySyncResponse
}

func (m *membershipHandler) membersGet(r *http.Request) response.Response {
	if m.service.Gateway().Cluster() == nil {
		return response.InternalError(errors.New("Cluster is still initializing"))
	}

	members, err := m.service.GetMembers(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	for i := range members {
		members[i].FileSets, err = m.mover.CountByTarget(r.Context(), members[i].Name)
		if err != nil {
			return response.SmartError(err)
		}
	}

	return response.SyncResponse(true, members)
}

func (m *membershipHandler) tokenPost(r *http.Request) response.Response {
	var req api.ClusterTokenPost

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	err = m.service.CreateToken(r.Context(), req)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) tokensGet(r *http.Request) response.Response {
	tokens, err := m.service.GetTokens(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(true, tokens.ToAPI())
}

func (m *membershipHandler) tokenGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	token, err := m.service.GetTokenByServerName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	tokenStr, err := token.ToAPI().String()
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(true, tokenStr)
}

func (m *membershipHandler) memberDelete(r *http.Request) response.Response {
	name := r.PathValue("name")
	force, _ := strconv.ParseBool(r.URL.Query().Get("force"))

	// The files only stored by the member would be lost.
	if !force {
		fileSets, err := m.mover.CountByTarget(r.Context(), name)
		if err != nil {
			return response.SmartError(err)
		}

		if fileSets > 0 {
			return response.SmartError(domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster member %q is the file target of %d file sets, which no other member stores", name, fileSets).
				WithHintf("Move the files to another cluster member first, or force the removal, which loses them.").
				WithDetail("member", name).
				WithDetail("file_sets", strconv.Itoa(fileSets)))
		}
	}

	err := m.service.RemoveMember(r.Context(), name, force)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) memberMoveFilesPost(r *http.Request) response.Response {
	var req api.ClusterMemberMoveFilesPost

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	err = m.mover.Move(r.Context(), r.PathValue("name"), req.Target)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) tokenDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := m.service.DeleteTokenByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
