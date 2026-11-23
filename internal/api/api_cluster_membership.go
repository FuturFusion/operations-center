package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	clusterSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type membershipHandler struct {
	authorizer *authz.Authorizer
	service    clusterSvc.MembershipService

	// restart restarts the daemon, so it comes up clustered.
	restart func()

	// serverSvc holds the server records of the members.
	serverSvc provisioning.ServerService
}

func registerClusterMembershipHandler(
	router Router,
	authorizer *authz.Authorizer,
	service clusterSvc.MembershipService,
	restart func(),
	serverSvc provisioning.ServerService,
) {
	handler := &membershipHandler{
		authorizer: authorizer,
		service:    service,
		restart:    restart,
		mover:      mover,
		serverSvc:  serverSvc,
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

	if req.ServerName != api.ServerNameOperationsCenter {
		err = m.checkServerNameUnused(r.Context(), req.ServerName)
		if err != nil {
			return response.SmartError(err)
		}
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

	err = m.checkServerNameUnused(r.Context(), req.ServerName)
	if err != nil {
		return response.SmartError(err)
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

	// A leader removing itself is removed by the new leader, which deletes the server record.
	removesItself, err := m.isLocalMember(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	ctx := r.Context()

	err = m.service.RemoveMember(ctx, name, force)
	if errors.Is(err, domain.ErrNotFound) {
		// A previous removal might have left the server record of the member behind.
		removed, removeErr := m.removeStaleMemberRecord(ctx, name)
		if removeErr != nil {
			return response.SmartError(removeErr)
		}

		if removed {
			return response.EmptySyncResponse
		}
	}

	if err != nil {
		return response.SmartError(err)
	}

	// Member and server names are distinct, so the record of this name belongs to the member.
	if !removesItself {
		err = m.serverSvc.DeleteByName(ctx, name)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return response.SmartError(fmt.Errorf("Failed to delete the server record of removed cluster member %q: %w", name, err))
		}
	}

	return response.EmptySyncResponse
}

// removeStaleMemberRecord deletes the Operations Center server record of name, which is no cluster member anymore.
func (m *membershipHandler) removeStaleMemberRecord(ctx context.Context, name string) (bool, error) {
	server, err := m.serverSvc.GetByName(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if server.Type != api.ServerTypeOperationsCenter {
		return false, nil
	}

	err = m.serverSvc.DeleteByName(ctx, name)
	if err != nil {
		return false, fmt.Errorf("Failed to delete the server record of removed cluster member %q: %w", name, err)
	}

	return true, nil
}

// isLocalMember reports whether name is this cluster member.
func (m *membershipHandler) isLocalMember(ctx context.Context, name string) (bool, error) {
	members, err := m.service.GetMembers(ctx)
	if err != nil {
		return false, err
	}

	localAddress := m.service.Gateway().State().ClusterAddress()
	for _, member := range members {
		if member.Name == name {
			return member.Address == localAddress, nil
		}
	}

	return false, nil
}

// checkServerNameUnused ensures the server name for the cluster member is not in use by a registered server.
func (m *membershipHandler) checkServerNameUnused(ctx context.Context, name string) error {
	_, err := m.serverSvc.GetByName(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("Failed to check the server name %q: %w", name, err)
	}

	return domain.NewErrorf(domain.ErrConstraintViolation, "", "The name %q is used by a server already", name).
		WithHintf("Use another name for the cluster member or rename the server.").
		WithDetail("server", name)
}

func (m *membershipHandler) tokenDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := m.service.DeleteTokenByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
