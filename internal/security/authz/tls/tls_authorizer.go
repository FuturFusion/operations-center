package authz

import (
	"context"
	"net/http"
	"strings"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// TLS represents a TLS authorizer.
type TLS struct {
	certificateFingerprints []string
}

var _ authz.Authorizer = TLS{}

func New(ctx context.Context, certificateFingerprints []string) TLS {
	return TLS{
		certificateFingerprints: certificateFingerprints,
	}
}

func (t TLS) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	if details.Protocol != api.AuthenticationMethodTLS {
		return api.StatusErrorf(http.StatusForbidden, "Authentication protocol %q, is not compatible with authorizer", details.Protocol)
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
