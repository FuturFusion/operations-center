package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/response"
)

func Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authentication
		trusted, username, protocol, _ := authenticate(w, r)

		if !trusted {
			slog.Warn("Rejecting request from untrusted client", slog.String("ip", r.RemoteAddr), slog.String("path", r.RequestURI), slog.String("method", r.Method))
			_ = response.Forbidden(nil).Render(w)
			return
		}

		slog.Debug("Handling API request", slog.String("method", r.Method), slog.String("url", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		// Add authentication/authorization context data.
		ctx := context.WithValue(r.Context(), CtxUsername, username)
		ctx = context.WithValue(ctx, CtxProtocol, protocol)

		r = r.WithContext(ctx)

		// Call next handler
		next(w, r)
	}
}

// authenticate validates an incoming http Request
// It will check over what protocol it came, what type of request it is and
// will validate the TLS certificate.
//
// This does not perform authorization, only validates authentication.
// Returns whether trusted or not, the username (or certificate fingerprint) of the trusted client, and the type of
// client that has been authenticated (unix or tls).
func authenticate(_ http.ResponseWriter, r *http.Request) (bool, string, string, error) {
	// Local unix socket queries.
	if r.RemoteAddr == "@" && r.TLS == nil {
		return true, "", "unix", nil
	}

	// Bad query, no TLS found.
	if r.TLS == nil {
		return false, "", "", fmt.Errorf("Bad/missing TLS on network query")
	}

	// Reject unauthorized.
	return false, "", "", nil
}
