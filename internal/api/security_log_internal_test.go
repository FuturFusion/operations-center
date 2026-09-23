package api

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	authzmiddleware "github.com/FuturFusion/operations-center/internal/security/authz/middleware"
	"github.com/FuturFusion/operations-center/internal/security/authz/unixsocket"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestAuthorizerErrorIsNotLoggedByTheDecorator(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", false, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", false, false, true), "the log levels must not leak into other tests")
	})

	authorizer := authzmiddleware.NewAuthorizerWithSlog(
		unixsocket.New(),
		authzmiddleware.AuthorizerWithSlogWithComponent(componentAuthzUnixSocket),
	)

	err := authorizer.CheckPermission(t.Context(), &authz.RequestDetails{Protocol: api.AuthenticationMethodTLS}, authz.ObjectServer(), authz.EntitlementCanView)

	require.Error(t, err, "the unix socket authorizer declines a request made over TLS")
	require.Empty(t, logBuf.String(), "an error returned to the caller must not be logged at the default level, it is reported by the boundary it ends up at")
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
	)

	_ = authorizer.CheckPermission(t.Context(), &authz.RequestDetails{Protocol: api.AuthenticationMethodTLS}, authz.ObjectServer(), authz.EntitlementCanView)

	require.Contains(t, logBuf.String(), "component=security.authz.unixsocket",
		"the level configured for the parent covers the individual authorizers")
}
