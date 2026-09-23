package authn

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/response"
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
