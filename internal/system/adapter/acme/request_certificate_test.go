package acme_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/system/adapter/acme"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestRequestCertificate(t *testing.T) {
	t.Parallel()

	unreachableCfg := system.SecurityACME{
		Domain:    "oc.example.com",
		Email:     "admin@example.com",
		CAURL:     "https://127.0.0.1:1/directory",
		Challenge: system.ACMEChallengeHTTP,
		Address:   "127.0.0.1:0",
		AgreeTOS:  true,
	}

	tests := []struct {
		name string
		cfg  system.SecurityACME

		setupCache func(t *testing.T, cacheDir string)

		assertErr require.ErrorAssertionFunc

		assertAcmeDir func(t *testing.T, acmeDir string)
	}{
		{
			name: "error - invalid CA URL",
			cfg: system.SecurityACME{
				Domain:   "oc.example.com",
				Email:    "admin@example.com",
				CAURL:    "not a url",
				AgreeTOS: true,
			},

			setupCache: func(t *testing.T, cacheDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to parse CA URL", a...)
			},

			assertAcmeDir: func(t *testing.T, acmeDir string) {
				t.Helper()
				require.NoDirExists(t, acmeDir, "acme directory is only created when needed")
			},
		},
		{
			name: "error - acme is not a directory",
			cfg:  unreachableCfg,

			setupCache: func(t *testing.T, cacheDir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "acme"), nil, 0o600))
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to open acme directory", a...)
			},

			assertAcmeDir: func(t *testing.T, acmeDir string) {
				t.Helper()

				info, err := os.Stat(acmeDir)
				require.NoError(t, err)
				require.False(t, info.IsDir(), "existing file must not be replaced")
			},
		},
		{
			name: "error - CA not reachable",
			cfg:  unreachableCfg,

			setupCache: func(t *testing.T, cacheDir string) {
				t.Helper()
			},

			assertErr: require.Error,

			assertAcmeDir: func(t *testing.T, acmeDir string) {
				t.Helper()

				info, err := os.Stat(acmeDir)
				require.NoError(t, err)
				require.True(t, info.IsDir(), "acme directory is created")
				require.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "acme directory must be private")
			},
		},
		{
			name: "error - CA not reachable, stale accounts are removed",
			cfg:  unreachableCfg,

			setupCache: func(t *testing.T, cacheDir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "acme", "accounts", "other-ca.example.com", "admin@example.com"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "acme", "accounts", "127.0.0.1", "other@example.com"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "acme", "certificates"), 0o700))
			},

			assertErr: require.Error,

			assertAcmeDir: func(t *testing.T, acmeDir string) {
				t.Helper()
				require.NoDirExists(t, filepath.Join(acmeDir, "accounts", "other-ca.example.com"), "account of other CA is removed")
				require.NoDirExists(t, filepath.Join(acmeDir, "accounts", "127.0.0.1", "other@example.com"), "account of other email is removed")
				require.NoDirExists(t, filepath.Join(acmeDir, "certificates"), "certificates are removed")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			cacheDir := t.TempDir()
			tc.setupCache(t, cacheDir)

			cacheRoot, err := os.OpenRoot(cacheDir)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, cacheRoot.Close()) })

			// The lego client retries for a long time. Stop it early.
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			t.Cleanup(cancel)

			// Run test
			got, err := acme.New(cacheRoot).RequestCertificate(ctx, tc.cfg)

			// Assert
			tc.assertErr(t, err)
			require.Nil(t, got, "no certificate is returned in these cases")
			tc.assertAcmeDir(t, filepath.Join(cacheDir, "acme"))
		})
	}
}
