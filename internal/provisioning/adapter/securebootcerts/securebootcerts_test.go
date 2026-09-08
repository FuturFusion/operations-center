package securebootcerts_test

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootcerts"
)

func TestNew(t *testing.T) {
	catalog, err := securebootcerts.New()
	require.NoError(t, err, "The shipped certificate catalog has to be valid")

	for _, fingerprint := range catalog.Fingerprints() {
		certificates, unknown := catalog.CertificatesByFingerprint([]string{fingerprint})
		require.Empty(t, unknown, "The catalog has to know its own fingerprint %q", fingerprint)
		require.Len(t, certificates, 1)

		require.Equal(t, fingerprint, fingerprintOf(t, certificates[0]), "The certificate has to be the one the fingerprint names")
		require.NotEqual(t, fingerprint, catalog.Describe(fingerprint), "Every certificate of the catalog has to have a description")
	}
}

func TestCatalog_CertificatesByFingerprint(t *testing.T) {
	catalog, err := securebootcerts.New()
	require.NoError(t, err)

	known := catalog.Fingerprints()

	certificates, unknown := catalog.CertificatesByFingerprint([]string{known[0], "unknown", known[1]})
	require.Len(t, certificates, 2, "The known certificates have to be resolved")
	require.Equal(t, []string{"unknown"}, unknown, "A fingerprint, that is not part of the catalog, has to be reported back")

	require.Equal(t, "unknown", catalog.Describe("unknown"), "An unknown fingerprint is described by itself")
}

func fingerprintOf(t *testing.T, pemCertificate string) string {
	t.Helper()

	block, _ := pem.Decode([]byte(pemCertificate))
	require.NotNil(t, block)

	certificate, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	sum := sha256.Sum256(certificate.Raw)

	return hex.EncodeToString(sum[:])
}
