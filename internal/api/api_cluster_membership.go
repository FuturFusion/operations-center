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
}

func registerClusterMembershipHandler(
	router Router,
	authorizer *authz.Authorizer,
	service clusterSvc.MembershipService,
	restart func(),
) {
	handler := &membershipHandler{
		authorizer: authorizer,
		service:    service,
		restart:    restart,
	}

	// Clustering:
	//
	// Enable clustering as the first member.
	router.HandleFunc("POST /:enable", response.With(handler.enablePost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanAdmin)))
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
