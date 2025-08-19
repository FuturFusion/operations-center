package system_test

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/system"
)

func TestSystemService_UpdateCertificate(t *testing.T) {
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tests := []struct {
		name     string
		setupEnv func(t *testing.T, targetDir string)
		certPEM  string
		keyPEM   string

		serverCertificateUpdateCallExpected bool
		assertError                         require.ErrorAssertionFunc
	}{
		{
			name: "success",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			serverCertificateUpdateCallExpected: true,
			assertError:                         require.NoError,
		},
		{
			name: "error - invalid certificate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: "invalid-cert",
			keyPEM:  "invalid-key",

			serverCertificateUpdateCallExpected: false,
			assertError: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to validate key pair")
			},
		},
		{
			name: "error - unable to write certificate file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.MkdirAll(filepath.Join(targetDir, "server.crt"), 0o000)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			serverCertificateUpdateCallExpected: false,
			assertError: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.crt")
			},
		},
		{
			name: "error - unable to write certificate key file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.MkdirAll(filepath.Join(targetDir, "server.key"), 0o000)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			serverCertificateUpdateCallExpected: false,
			assertError: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.key")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			env := mockEnv{
				varDir: tmpDir,
			}

			tc.setupEnv(t, env.VarDir())

			serverCertificateUpdateResp := make(chan struct{}, 1)

			serverCertificateUpdate := signals.NewSync[tls.Certificate]()
			serverCertificateUpdate.AddListener(func(ctx context.Context, cert tls.Certificate) {
				serverCertificateUpdateResp <- struct{}{}
			})

			systemSvc := system.NewSystemService(env, serverCertificateUpdate)

			err = systemSvc.UpdateCertificate(context.Background(), tc.certPEM, tc.keyPEM)
			tc.assertError(t, err)

			serverCertificateUpdateCalled := false
			select {
			case <-serverCertificateUpdateResp:
				serverCertificateUpdateCalled = true
			case <-time.After(10 * time.Millisecond):
			}

			require.Equal(t, tc.serverCertificateUpdateCallExpected, serverCertificateUpdateCalled)
		})
	}
}

type mockEnv struct {
	varDir string
}

func (e mockEnv) VarDir() string { return e.varDir }
