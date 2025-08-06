package api

import (
	"context"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
)

type dummyAuthenticator struct{}

func (d dummyAuthenticator) Auth(w http.ResponseWriter, r *http.Request) (bool, string, string, error) {
	return true, "testuser", "testprotocol", nil
}

type noopAuthorizer struct{}

func (n noopAuthorizer) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	return nil
}

type MockEnv struct {
	LogDirectory string
	VarDirectory string
	UnixSocket   string
}

func (e MockEnv) LogDir() string        { return e.LogDirectory }
func (e MockEnv) VarDir() string        { return e.VarDirectory }
func (e MockEnv) GetUnixSocket() string { return e.UnixSocket }
