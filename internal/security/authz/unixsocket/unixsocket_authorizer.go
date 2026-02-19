package unixsocket

import (
	"context"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// UnixSocket represents an unix socket authorizer.
type UnixSocket struct{}

var _ authz.Authorizer = UnixSocket{}

func New() UnixSocket {
	return UnixSocket{}
}

func (u UnixSocket) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	// Always allow full access via local unix socket.
	if details.Protocol == "unix" {
		return nil
	}

	return api.StatusErrorf(http.StatusForbidden, "User is not connected through unixsocket")
}
