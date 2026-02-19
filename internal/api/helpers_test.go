package api

import (
	"context"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authz"
)

type dummyAuthenticator struct{}

func (d dummyAuthenticator) Auth(w http.ResponseWriter, r *http.Request) (bool, string, string, error) {
	return true, "testuser", "testprotocol", nil
}

type noopAuthorizer struct{}

func (n noopAuthorizer) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	return nil
}
