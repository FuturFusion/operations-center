package authz

import (
	"context"
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

func New(ctx context.Context, certificateFingerprints []string) TLS {
	return TLS{
		certificateFingerprints: certificateFingerprints,
	}
}

func (t TLS) CheckPermission(ctx context.Context, r *http.Request, object authz.Object, entitlement authz.Entitlement) error {
	// TODO: This should not be necessary in every authorizer again and again
	details, err := t.RequestDetails(r)
	if err != nil {
		return api.StatusErrorf(http.StatusForbidden, "Failed to extract request details: %v", err)
	}

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
