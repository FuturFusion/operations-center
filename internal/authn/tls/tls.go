package tls

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"

	"github.com/FuturFusion/operations-center/internal/authn"
	"github.com/FuturFusion/operations-center/shared/api"
)

type TLS struct {
	trustedTLSClientCertFingerprints []string
}

var _ authn.Auther = TLS{}

func New(trustedTLSClientCertFingerprints []string) TLS {
	return TLS{
		trustedTLSClientCertFingerprints: trustedTLSClientCertFingerprints,
	}
}

func (t TLS) Auth(w http.ResponseWriter, r *http.Request) (trusted bool, username string, protocol string, _ error) {
	// Bad query, no TLS found.
	if r.TLS == nil {
		return false, "", "", errors.New("Bad/missing TLS on network query")
	}

	for _, cert := range r.TLS.PeerCertificates {
		trusted, username := checkTrustState(r.Context(), *cert, t.trustedTLSClientCertFingerprints)
		if trusted {
			return true, username, api.AuthenticationMethodTLS, nil
		}
	}

	// Reject unauthorized.
	return false, "", "", nil
}

// checkTrustState checks whether the given client certificate is trusted
// (i.e. it has a valid time span and it belongs to the given list of trusted
// certificates).
// Returns whether or not the certificate is trusted, and the fingerprint of the certificate.
func checkTrustState(ctx context.Context, cert x509.Certificate, trustedCertFingerprints []string) (bool, string) {
	// Extra validity check (should have been caught by TLS stack)
	if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
		return false, ""
	}

	certFingerprint := incustls.CertFingerprint(&cert)

	// Check whether client certificate fingerprint is trusted.
	for _, fingerprint := range trustedCertFingerprints {
		canonicalFingerprint := strings.ToLower(strings.ReplaceAll(fingerprint, ":", ""))
		if certFingerprint == canonicalFingerprint {
			slog.DebugContext(ctx, "Matched trusted cert", slog.String("fingerprint", canonicalFingerprint), slog.Any("subject", cert.Subject))
			return true, canonicalFingerprint
		}
	}

	return false, ""
}
