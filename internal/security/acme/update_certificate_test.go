package acme_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type varDirEnv struct {
	dir string
}

func (e varDirEnv) VarDir() string {
	return e.dir
}

func TestUpdateCertificate(t *testing.T) {
	t.Parallel()

	enabledCfg := system.SecurityACME{
		Domain:   "oc.example.com",
		Email:    "admin@example.com",
		CAURL:    "https://ca.example.com/directory",
		AgreeTOS: true,
	}

	tests := []struct {
		name  string
		cfg   system.SecurityACME
		force bool
		setup func(t *testing.T, varDir string)

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "ACME disabled",
			cfg:   system.SecurityACME{},
			force: false,
			setup: func(t *testing.T, varDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
		},
		{
			name:  "certificate not due for renewal",
			cfg:   enabledCfg,
			force: false,
			setup: func(t *testing.T, varDir string) {
				t.Helper()
				requireWriteServerKeyPair(t, varDir, "oc.example.com")
			},

			assertErr: require.NoError,
		},
		{
			name:  "error - invalid certificate file",
			cfg:   enabledCfg,
			force: false,
			setup: func(t *testing.T, varDir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(varDir, "server.crt"), []byte("invalid"), 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(varDir, "server.key"), []byte("invalid"), 0o600))
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to load certificate and key file", a...)
			},
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
			setup: func(t *testing.T, varDir string) {
				t.Helper()
				requireWriteServerKeyPair(t, varDir, "oc.example.com")
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
			varDir := t.TempDir()
			tc.setup(t, varDir)

			acmeRoot, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, acmeRoot.Close()) })

			// Run test
			got, err := acme.UpdateCertificate(t.Context(), varDirEnv{dir: varDir}, acmeRoot, tc.cfg, tc.force)

			// Assert
			tc.assertErr(t, err)
			require.Nil(t, got, "no certificate is requested in these cases")
		})
	}
}

func requireWriteServerKeyPair(t *testing.T, dir string, dnsName string) {
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

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
}
