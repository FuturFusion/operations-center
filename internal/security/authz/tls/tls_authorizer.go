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
	certificateFingerprints []func() []string
}

var _ authz.Authorizer = TLS{}

func New(ctx context.Context, certificateFingerprints ...func() []string) TLS {
	return TLS{
		certificateFingerprints: certificateFingerprints,
	}
}

func (t TLS) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	if details.Protocol != api.AuthenticationMethodTLS {
		return api.StatusErrorf(http.StatusForbidden, "Authentication protocol %q, is not compatible with authorizer", details.Protocol)
	}

	for _, fpSet := range t.certificateFingerprints {
		for _, fingerprint := range fpSet() {
			canonicalFingerprint := strings.ToLower(strings.ReplaceAll(fingerprint, ":", ""))
			if canonicalFingerprint == details.Username {
				// Authentication via TLS gives full, unrestricted access.
				return nil
			}
		}
	}

	return api.StatusErrorf(http.StatusForbidden, "Client certificate not found")
}
