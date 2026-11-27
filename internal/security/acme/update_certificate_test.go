package acme_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestUpdateCertificate(t *testing.T) {
	t.Parallel()

	enabledCfg := system.SecurityACME{
		Domain:   "oc.example.com",
		Email:    "admin@example.com",
		CAURL:    "https://ca.example.com/directory",
		AgreeTOS: true,
	}

	tests := []struct {
		name    string
		cfg     system.SecurityACME
		force   bool
		current func(t *testing.T) *x509.Certificate

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "ACME disabled",
			cfg:   system.SecurityACME{},
			force: false,
			current: func(t *testing.T) *x509.Certificate {
				t.Helper()
				return nil
			},

			assertErr: require.NoError,
		},
		{
			name:  "certificate not due for renewal",
			cfg:   enabledCfg,
			force: false,
			current: func(t *testing.T) *x509.Certificate {
				t.Helper()
				return requireCertificate(t, "oc.example.com")
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid CA URL",
			cfg: system.SecurityACME{
				Domain:   "oc.example.com",
				Email:    "admin@example.com",
				CAURL:    "not a url",
				AgreeTOS: true,
			},
			force: true,
			current: func(t *testing.T) *x509.Certificate {
				t.Helper()
				return requireCertificate(t, "oc.example.com")
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to parse CA URL", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			acmeRoot, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, acmeRoot.Close()) })

			// Run test
			got, err := acme.UpdateCertificate(t.Context(), acmeRoot, tc.cfg, tc.current(t), tc.force)

			// Assert
			tc.assertErr(t, err)
			require.Nil(t, got, "no certificate is requested in these cases")
		})
	}
}

func requireCertificate(t *testing.T, dnsName string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert
}
