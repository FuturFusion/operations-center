package unixsocket

import (
	"context"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// UnixSocket represents an unix socket authorizer.
type UnixSocket struct {
	authz.CommonAuthorizer
}

func New() UnixSocket {
	return UnixSocket{}
}

func (u UnixSocket) CheckPermission(ctx context.Context, r *http.Request, object authz.Object, entitlement authz.Entitlement) error {
	// TODO: This should not be necessary in every authorizer again and again
	details, err := u.RequestDetails(r)
	if err != nil {
		return api.StatusErrorf(http.StatusForbidden, "Failed to extract request details: %v", err)
	}

	// Always allow full access via local unix socket.
	if details.Protocol == "unix" {
		return nil
	}

	return api.StatusErrorf(http.StatusForbidden, "User is not connected through unixsocket")
}
