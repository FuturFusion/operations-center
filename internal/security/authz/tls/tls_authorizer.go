package authz

import (
	"context"
	"net/http"
	"slices"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/shared/api"
)

// TLS represents a TLS authorizer.
type TLS struct {
	certificateFingerprints        []string
	metricsCertificateFingerprints []string
}

var _ authz.Authorizer = TLS{}

// Option configures the TLS authorizer.
type Option func(*TLS)

// WithMetricsCertificateFingerprints sets the fingerprints of the certificates
// that are restricted to metrics access.
func WithMetricsCertificateFingerprints(fingerprints []string) Option {
	return func(t *TLS) {
		t.metricsCertificateFingerprints = fingerprints
	}
}

func New(ctx context.Context, certificateFingerprints []string, opts ...Option) TLS {
	t := TLS{
		certificateFingerprints: certificateFingerprints,
	}

	for _, opt := range opts {
		opt(&t)
	}

	return t
}

func (t TLS) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	if details.Protocol != api.AuthenticationMethodTLS {
		return api.StatusErrorf(http.StatusForbidden, "Authentication protocol %q, is not compatible with authorizer", details.Protocol)
	}

	if containsFingerprint(t.certificateFingerprints, details.Username) {
		// A trusted certificate has full access. This check runs first, so full access wins.
		return nil
	}

	if containsFingerprint(t.metricsCertificateFingerprints, details.Username) {
		if entitlement == authz.EntitlementCanViewMetrics {
			return nil
		}

		// Stop the chain, so no other authorizer widens the access.
		return authz.Deny(api.StatusErrorf(http.StatusForbidden, "Client certificate is restricted to metrics access"))
	}

	return api.StatusErrorf(http.StatusForbidden, "Client certificate not found")
}

func containsFingerprint(fingerprints []string, username string) bool {
	return slices.ContainsFunc(fingerprints, func(fingerprint string) bool {
		return securitytls.CanonicalFingerprint(fingerprint) == username
	})
}
