package oidc

import (
	"context"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// OIDC represents an Open ID Connect authorizer where every user with
// valid OIDC credentials is granted unrestricted access.
type OIDC struct{}

var _ authz.Authorizer = OIDC{}

func New() OIDC {
	return OIDC{}
}

func (o OIDC) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	if details.Protocol != api.AuthenticationMethodOIDC {
		return api.StatusErrorf(http.StatusForbidden, "Authentication protocol %q, is not compatible with authorizer", details.Protocol)
	}

	return nil
}
