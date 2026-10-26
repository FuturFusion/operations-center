package authn

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

var componentAuthn = logger.RegisterComponent("security.authn")

type MiddlewareOption func(c *middlewareConfig)

type middlewareConfig struct {
	isAuthenticationRequired func(r *http.Request) bool
}

func WithIsAuthenticationRequired(isAuthenticationRequired func(r *http.Request) bool) MiddlewareOption {
	return func(c *middlewareConfig) {
		c.isAuthenticationRequired = isAuthenticationRequired
	}
}

// Middleware returns a http handler middleware, which will try to authenticate
// a request by probing all provided authers in sequence.
// When successful, the authenticated username and the protocol of the
// authentication is stored in the request context.
// For requests, which are not authenticated, only the authentication status is
// stored in the request context.
func (a *Authenticator) Middleware(opts ...MiddlewareOption) func(next http.HandlerFunc) http.HandlerFunc {
	cfg := &middlewareConfig{
		isAuthenticationRequired: func(r *http.Request) bool { return true },
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var username, protocol string
			var trusted bool
			var err error

			ctx := r.Context()

			// The component is only applied to the records emitted here. The
			// context handed to the next handler keeps the component of the
			// caller, so the whole request is not attributed to authn.
			logCtx := logger.ContextWithComponent(ctx, componentAuthn)

			// Authentication
			for _, auther := range a.authers {
				trusted, username, protocol, err = auther.Auth(w, r)
				if err != nil {
					slog.WarnContext(logCtx, "Authentication failed", logger.Err(err), slog.String("ip", r.RemoteAddr), slog.String("path", r.RequestURI), slog.String("method", r.Method))

					err = response.Unauthorized(err).Render(w)
					if err != nil {
						slog.WarnContext(logCtx, "Render error response failed", logger.Err(err))
					}

					return
				}

				if trusted {
					break
				}
			}

			// While starting, the identity forwarded by a cluster member can not be authorized yet.
			if trusted && a.starting && r.Header.Get(HeaderForwardedAddress) != "" {
				renderStarting(logCtx, w, r)
				return
			}

			// A cluster member forwarding a request passes on the identity of its client, which is not authenticated without one.
			if trusted && a.checkTrustedForward != nil && r.Header.Get(HeaderForwardedAddress) != "" && a.checkTrustedForward(r) {
				username = r.Header.Get(HeaderForwardedUsername)
				protocol = r.Header.Get(HeaderForwardedProtocol)
				// Clients on the unix socket have no username.
				trusted = protocol != ""
				ctx = context.WithValue(ctx, CtxForwardedAddress, r.Header.Get(HeaderForwardedAddress))

				cert, err := forwardedCertificate(r)
				if err != nil {
					err = response.BadRequest(err).Render(w)
					if err != nil {
						slog.WarnContext(logCtx, "Render error response failed", logger.Err(err))
					}

					return
				}

				if cert != nil {
					ctx = context.WithValue(ctx, CtxForwardedCertificate, cert)
				}
			}

			if cfg.isAuthenticationRequired(r) && !trusted && a.starting {
				renderStarting(logCtx, w, r)
				return
			}

			if cfg.isAuthenticationRequired(r) && !trusted {
				slog.WarnContext(logCtx, "Rejecting request from untrusted client", slog.String("ip", r.RemoteAddr), slog.String("path", r.RequestURI), slog.String("method", r.Method))
				err = response.Unauthorized(nil).Render(w)
				if err != nil {
					slog.WarnContext(logCtx, "Render forbidden response failed", logger.Err(err))
				}

				return
			}

			slog.DebugContext(logCtx, "Handling API request", slog.String("method", r.Method), slog.String("url", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

			// Add authentication/authorization context data.
			ctx = context.WithValue(ctx, CtxAuthenticated, trusted)
			if trusted {
				ctx = context.WithValue(ctx, CtxUsername, username)
				ctx = context.WithValue(ctx, CtxProtocol, protocol)
			}

			r = r.WithContext(ctx)

			// Call next handler
			next(w, r)
		}
	}
}

// renderStarting answers a request that can not be authenticated before the security configuration is loaded.
func renderStarting(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	slog.DebugContext(ctx, "Rejecting request while starting", slog.String("ip", r.RemoteAddr), slog.String("path", r.RequestURI), slog.String("method", r.Method))

	err := response.SmartError(api.StatusErrorf(http.StatusTooEarly, "Operations Center is still starting")).Render(w)
	if err != nil {
		slog.WarnContext(ctx, "Render error response failed", logger.Err(err))
	}
}

// forwardedCertificate returns the client certificate passed on by a cluster member, nil if there is none.
func forwardedCertificate(r *http.Request) (*x509.Certificate, error) {
	value := r.Header.Get(HeaderForwardedCertificate)
	if value == "" {
		return nil, nil
	}

	der, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("Invalid forwarded client certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("Invalid forwarded client certificate: %w", err)
	}

	return cert, nil
}
