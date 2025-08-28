package authn

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/response"
)

// Middleware returns a http handler middleware, which will try to authenticate
// a request by probing all provided authers in sequence.
// When successful, the authenticated username and the protocol of the
// authentication is stored in the request context.
func (a *Authenticator) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var username, protocol string
		var trusted bool
		var err error

		ctx := r.Context()

		// Authentication
		for _, auther := range a.authers {
			trusted, username, protocol, err = auther.Auth(w, r)
			if err != nil {
				err = response.Unauthorized(err).Render(w)
				if err != nil {
					slog.WarnContext(ctx, "Render error response failed", logger.Err(err))
				}

				return
			}

			if trusted {
				break
			}
		}

		if !trusted {
			slog.WarnContext(ctx, "Rejecting request from untrusted client", slog.String("ip", r.RemoteAddr), slog.String("path", r.RequestURI), slog.String("method", r.Method))
			err = response.Forbidden(nil).Render(w)
			if err != nil {
				slog.WarnContext(ctx, "Render forbidden response failed", logger.Err(err))
			}

			return
		}

		slog.DebugContext(ctx, "Handling API request", slog.String("method", r.Method), slog.String("url", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		// Add authentication/authorization context data.
		ctx = context.WithValue(ctx, CtxUsername, username)
		ctx = context.WithValue(ctx, CtxProtocol, protocol)

		r = r.WithContext(ctx)

		// Call next handler
		next(w, r)
	}
}
