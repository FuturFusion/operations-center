package tls

import (
	"context"
	"crypto/x509"
	"log/slog"
	"strings"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"
)

// CheckTrustState checks whether the given client certificate is trusted
// (i.e. it has a valid time span and it belongs to the given list of trusted
// certificates).
// Returns whether or not the certificate is trusted, and the fingerprint of the certificate.
func CheckTrustState(ctx context.Context, cert x509.Certificate, trustedCertFingerprints []string) (bool, string) {
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
