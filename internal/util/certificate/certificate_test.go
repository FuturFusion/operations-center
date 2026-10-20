package certificate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/testing/certs"
)

func TestDecode(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			cert, err := certificate.Decode([]byte(tc.certPEM))

			tc.assertErr(t, err)
			if err != nil {
				return
			}

			require.NotNil(t, cert)
		})
	}
}

func TestDERFingerprint(t *testing.T) {
	t.Parallel()

	leafPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	issuerPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	leafBlock, _ := pem.Decode(leafPEM)
	require.NotNil(t, leafBlock)

	leafSum := sha256.Sum256(leafBlock.Bytes)
	leafFingerprint := hex.EncodeToString(leafSum[:])

	tests := []struct {
		name           string
		pemCertificate string

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:           "success",
			pemCertificate: string(leafPEM),

			want:      leafFingerprint,
			assertErr: require.NoError,
		},
		{
			name:           "success - a chain is identified by its leaf certificate",
			pemCertificate: string(leafPEM) + string(issuerPEM),

			want:      leafFingerprint,
			assertErr: require.NoError,
		},
		{
			name:           "error - empty",
			pemCertificate: "",

			assertErr: require.Error,
		},
		{
			name:           "error - not PEM encoded",
			pemCertificate: "not a PEM encoded certificate",

			assertErr: require.Error,
		},
		{
			name:           "error - PEM block is not a certificate",
			pemCertificate: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("public key")})),

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := certificate.DERFingerprint("test", []byte(tc.pemCertificate))

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDERFingerprintDoesNotParse(t *testing.T) {
	t.Parallel()

	unparseable := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-a-real-der-certificate")}))

	_, err := certificate.Decode([]byte(unparseable))
	require.Error(t, err, "The certificate has to be one x509.ParseCertificate rejects")

	sum := sha256.Sum256([]byte("not-a-real-der-certificate"))

	got, err := certificate.DERFingerprint("unparseable", []byte(unparseable))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(sum[:]), got)
}

func TestEncodeToPEM(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
			certPEM:         certs.GenerateWithValidity(t, now.Add(-24*time.Hour), now.Add(365*24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: true,
			assertErr: require.NoError,
		},
		{
			name:            "valid - expires within threshold",
			certPEM:         certs.GenerateWithValidity(t, now.Add(-24*time.Hour), now.Add(10*24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: true,
			assertErr: require.Error,
		},
		{
			name:            "invalid - not yet valid",
			certPEM:         certs.GenerateWithValidity(t, now.Add(24*time.Hour), now.Add(365*24*time.Hour)),
			expiryThreshold: 30 * 24 * time.Hour,

			wantValid: false,
			assertErr: require.Error,
		},
		{
			name:            "invalid - expired",
			certPEM:         certs.GenerateWithValidity(t, now.Add(-365*24*time.Hour), now.Add(-24*time.Hour)),
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

func TestValidateLifetime(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name          string
		certPEM       string
		lifetimeRatio float64

		wantValid bool
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success - 61 of 90 days passed",
			certPEM:       certs.GenerateWithValidity(t, now.Add(-61*24*time.Hour), now.Add(29*24*time.Hour)),
			lifetimeRatio: 0.9,

			wantValid: true,
			assertErr: require.NoError,
		},
		{
			name:          "valid - 82 of 90 days passed",
			certPEM:       certs.GenerateWithValidity(t, now.Add(-82*24*time.Hour), now.Add(8*24*time.Hour)),
			lifetimeRatio: 0.9,

			wantValid: true,
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "expires within", a...)
			},
		},
		{
			name:          "invalid - expired",
			certPEM:       certs.GenerateWithValidity(t, now.Add(-91*24*time.Hour), now.Add(-24*time.Hour)),
			lifetimeRatio: 0.9,

			wantValid: false,
			assertErr: require.Error,
		},
		{
			name:          "invalid - not PEM encoded",
			certPEM:       "invalid",
			lifetimeRatio: 0.9,

			wantValid: false,
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid, err := certificate.ValidateLifetime(tc.certPEM, tc.lifetimeRatio)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValid, valid, "Validity of the certificate")
		})
	}
}
