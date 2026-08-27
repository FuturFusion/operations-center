package certificate_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/certificate"
)

// newCertPEM returns a PEM encoded certificate with the given validity period.
func newCertPEM(t *testing.T, notBefore time.Time, notAfter time.Time) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "operations-center"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	return certificate.EncodeToPEM(der)
}

func TestDecode(t *testing.T) {
	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	malformedDER := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-a-real-der-certificate")}))

	tests := []struct {
		name    string
		certPEM string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			certPEM: string(certPEM),

			assertErr: require.NoError,
		},
		{
			name:    "error - not PEM encoded",
			certPEM: "invalid",

			assertErr: require.Error,
		},
		{
			name:    "error - PEM decodes but is not a valid X509 certificate",
			certPEM: malformedDER,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cert, err := certificate.Decode([]byte(tc.certPEM))

			tc.assertErr(t, err)
			if err != nil {
				return
			}

			require.NotNil(t, cert)
		})
	}
}

func TestEncodeToPEM(t *testing.T) {
	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := certificate.Decode(certPEM)
	require.NoError(t, err)

	// Round trip through encode and decode returns the same certificate.
	roundTripped, err := certificate.Decode([]byte(certificate.EncodeToPEM(cert.Raw)))
	require.NoError(t, err)
	require.Equal(t, cert.Raw, roundTripped.Raw)

	require.Equal(t, certificate.EncodeToPEM(cert.Raw), certificate.X509EncodeToPEM(cert))
	require.Empty(t, certificate.X509EncodeToPEM(nil))
}

func TestValidate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		certPEM         string
		expiryThreshold time.Duration

		wantValid bool
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "success",
			certPEM:         newCertPEM(t, now.Add(-24*time.Hour), now.Add(365*24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: true,
			assertErr: require.NoError,
		},
		{
			name:            "valid - expires within threshold",
			certPEM:         newCertPEM(t, now.Add(-24*time.Hour), now.Add(10*24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: true,
			assertErr: require.Error,
		},
		{
			name:            "invalid - not yet valid",
			certPEM:         newCertPEM(t, now.Add(24*time.Hour), now.Add(365*24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: false,
			assertErr: require.Error,
		},
		{
			name:            "invalid - expired",
			certPEM:         newCertPEM(t, now.Add(-365*24*time.Hour), now.Add(-24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: false,
			assertErr: require.Error,
		},
		{
			name:            "invalid - not PEM encoded",
			certPEM:         "invalid",
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: false,
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			valid, err := certificate.Validate(tc.certPEM, tc.expiryThreshold)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValid, valid)
		})
	}
}
