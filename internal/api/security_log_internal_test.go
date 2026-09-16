package api

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	authnoidc "github.com/FuturFusion/operations-center/internal/security/authn/oidc"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	authzmiddleware "github.com/FuturFusion/operations-center/internal/security/authz/middleware"
	"github.com/FuturFusion/operations-center/internal/security/authz/unixsocket"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestAuthorizerDeclineIsInformative(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", false, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", false, false, true), "the log levels must not leak into other tests")
	})

	authorizer := authzmiddleware.NewAuthorizerWithSlog(
		unixsocket.New(),
		authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzUnixSocket),
		authzmiddleware.AuthorizerWithSlogWithInformativeErrFunc(isAuthzDeniedErr),
	)

	err := authorizer.CheckPermission(t.Context(), &authz.RequestDetails{Protocol: api.AuthenticationMethodTLS}, authz.ObjectServer(), authz.EntitlementCanView)

	require.Error(t, err, "the unix socket authorizer declines a request made over TLS")
	require.Empty(t, logBuf.String(), "an authorizer declining a protocol it does not serve must not be logged at the default level")

	withoutInformativeErrFunc := authzmiddleware.NewAuthorizerWithSlog(
		unixsocket.New(),
		authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzUnixSocket),
	)

	err = withoutInformativeErrFunc.CheckPermission(t.Context(), &authz.RequestDetails{Protocol: api.AuthenticationMethodTLS}, authz.ObjectServer(), authz.EntitlementCanView)

	require.Error(t, err)
	require.Contains(t, logBuf.String(), "returned an error", "without the informative error func the decline is logged as an error, which is what the informative error func prevents")
}

func TestIsAuthnFailedErr(t *testing.T) {
	require.True(t, isAuthnFailedErr(&authnoidc.AuthError{Err: errors.New("expired token")}), "a rejected credential is the regular negative answer of an authenticator")
	require.True(t, isAuthnFailedErr(fmt.Errorf("wrapped: %w", &authnoidc.AuthError{Err: errors.New("expired token")})), "a wrapped rejected credential is still informative")
	require.False(t, isAuthnFailedErr(errors.New("boom")), "a failure of the authenticator itself is not informative")
}

func TestIsAuthzDeniedErr(t *testing.T) {
	require.True(t, isAuthzDeniedErr(api.StatusErrorf(403, "denied")), "a rejection is the regular negative answer of an authorizer")
	require.True(t, isAuthzDeniedErr(api.StatusErrorf(401, "unauthorized")), "a missing credential is the regular negative answer of an authorizer")
	require.False(t, isAuthzDeniedErr(api.StatusErrorf(500, "boom")), "a server side failure is not informative")
	require.False(t, isAuthzDeniedErr(errors.New("boom")), "an error without status is not informative")
}

func TestAuthComponentDefaultsAreReachable(t *testing.T) {
	require.Contains(t, logger.Components(), logger.Component("security.authn"), "the component the auther decorator defaults to is registered")
	require.Contains(t, logger.Components(), logger.Component("security.authz"), "the component the authorizer decorator defaults to is registered")

	require.NoError(t, logger.ValidateComponentLevelsKnown(map[string]string{
		"security.authn": "DEBUG",
		"security.authz": "DEBUG",
	}), "the components the auth decorators default to are configurable")
}

func TestAuthorizerParentComponentCoversMembers(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", false, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", false, false, true), "the log levels must not leak into other tests")
	})

	require.NoError(t, logger.SetComponentLevels(map[string]slog.Level{"security.authz": slog.LevelDebug}),
		"setting the component levels must not fail")

	authorizer := authzmiddleware.NewAuthorizerWithSlog(
		unixsocket.New(),
		authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzUnixSocket),
		authzmiddleware.AuthorizerWithSlogWithInformativeErrFunc(isAuthzDeniedErr),
	)

	_ = authorizer.CheckPermission(t.Context(), &authz.RequestDetails{Protocol: api.AuthenticationMethodTLS}, authz.ObjectServer(), authz.EntitlementCanView)

	require.Contains(t, logBuf.String(), "component=security.authz.unixsocket",
		"the level configured for the parent covers the individual authorizers")
}
