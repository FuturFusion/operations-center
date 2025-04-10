package authn

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (a Authenticator) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authentication
		trusted, username, protocol, _ := a.authenticate(w, r)

		if !trusted {
			slog.WarnContext(r.Context(), "Rejecting request from untrusted client", slog.String("ip", r.RemoteAddr), slog.String("path", r.RequestURI), slog.String("method", r.Method))
			_ = response.Forbidden(nil).Render(w)
			return
		}

		slog.DebugContext(r.Context(), "Handling API request", slog.String("method", r.Method), slog.String("url", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		// Add authentication/authorization context data.
		ctx := context.WithValue(r.Context(), CtxUsername, username)
		ctx = context.WithValue(ctx, CtxProtocol, protocol)

		r = r.WithContext(ctx)

		// Call next handler
		next(w, r)
	}
}

// authenticate validates an incoming http Request.
// It will check over what protocol it came, what type of request it is and
// will validate the TLS certificate.
//
// This does not perform authorization, only validates authentication.
// Returns whether trusted or not, the username (or certificate fingerprint) of the trusted client, and the type of
// client that has been authenticated (unix or tls).
func (a Authenticator) authenticate(_ http.ResponseWriter, r *http.Request) (bool, string, string, error) { //nolint:unparam
	// Local unix socket queries.
	if r.RemoteAddr == "@" && r.TLS == nil {
		return true, "", "unix", nil
	}

	// Bad query, no TLS found.
	if r.TLS == nil {
		return false, "", "", fmt.Errorf("Bad/missing TLS on network query")
	}

	for _, cert := range r.TLS.PeerCertificates {
		trusted, username := checkTrustState(r.Context(), *cert, a.trustedTLSClientCertFingerprints)
		if trusted {
			return true, username, api.AuthenticationMethodTLS, nil
		}
	}

	// Reject unauthorized.
	return false, "", "", nil
}
