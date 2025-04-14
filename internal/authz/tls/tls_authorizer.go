package authz

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// TLS represents a TLS authorizer.
type TLS struct {
	authz.CommonAuthorizer

	certificateFingerprints []string
}

func New(ctx context.Context, certificateFingerprints []string) authz.Authorizer {
	return TLS{
		certificateFingerprints: certificateFingerprints,
	}
}

func (t TLS) CheckPermission(ctx context.Context, r *http.Request, object authz.Object, entitlement authz.Entitlement) error {
	logger := slog.With(slog.String("authorizer", "tls"))
	details, err := t.RequestDetails(r)
	if err != nil {
		return api.StatusErrorf(http.StatusForbidden, "Failed to extract request details: %v", err)
	}

	// Always allow full access via local unix socket.
	if details.Protocol == "unix" {
		return nil
	}

	if details.Protocol != api.AuthenticationMethodTLS {
		logger.WarnContext(ctx, "Authentication protocol is not compatible with authorizer", slog.String("protocol", details.Protocol))
		// Return nil. If the server has been configured with an authentication method but no associated authorizer,
		// the default is to give these authenticated users admin privileges.
		return nil
	}

	for _, fingerprint := range t.certificateFingerprints {
		canonicalFingerprint := strings.ToLower(strings.ReplaceAll(fingerprint, ":", ""))
		if canonicalFingerprint == details.Username {
			// Authentication via TLS gives full, unrestricted access.
			return nil
		}
	}

	return api.StatusErrorf(http.StatusForbidden, "Client certificate not found")
}
