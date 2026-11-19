package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

func Test_startingAuthorizer(t *testing.T) {
	t.Parallel()

	replaceable := authz.NewReplaceable(startingAuthorizer{})

	var authorizer authz.Authorizer = replaceable

	authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

	serveMux := http.NewServeMux()
	router := newRouter(serveMux).AddMiddlewares(authenticator.Middleware())
	router.HandleFunc("GET /files", response.With(func(r *http.Request) response.Response {
		return response.EmptySyncResponse
	}, assertPermission(&authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))

	get := func() int {
		rec := httptest.NewRecorder()
		serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files", nil))

		return rec.Code
	}

	// A trusted request while the security configuration is not loaded yet is asked to retry, not refused.
	require.Equal(t, http.StatusTooEarly, get())

	replaceable.Replace(noopAuthorizer{})
	require.Equal(t, http.StatusOK, get())
}
