package localfs

import (
	"os"
	"path/filepath"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestCertificate_GetServerKeyPair(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)

		assertErr          require.ErrorAssertionFunc
		wantCertificatePEM string
		wantKeyPEM         string
	}{
		{
			name: "success",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireWriteFile(t, filepath.Join(dir, "server.crt"), "certificate")
				requireWriteFile(t, filepath.Join(dir, "server.key"), "key")
			},

			assertErr:          require.NoError,
			wantCertificatePEM: "certificate",
			wantKeyPEM:         "key",
		},
		{
			name: "error - certificate missing",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireWriteFile(t, filepath.Join(dir, "server.key"), "key")
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, os.ErrNotExist, a...)
				require.ErrorContains(tt, err, `Failed to read "server.crt"`, a...)
			},
		},
		{
			name: "error - key missing",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireWriteFile(t, filepath.Join(dir, "server.crt"), "certificate")
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, os.ErrNotExist, a...)
				require.ErrorContains(tt, err, `Failed to read "server.key"`, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			dir := t.TempDir()
			tc.setup(t, dir)
			repo := NewCertificate(fsroot.Open(t, dir))

			// Run test
			certificatePEM, keyPEM, err := repo.GetServerKeyPair(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantCertificatePEM, string(certificatePEM))
			require.Equal(t, tc.wantKeyPEM, string(keyPEM))
		})
	}
}

func TestCertificate_UpdateServerKeyPair(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)

		assertErr          require.ErrorAssertionFunc
		wantCertificatePEM string
		assertKey          func(t *testing.T, keyFile string)
		wantEntries        []string
	}{
		{
			name: "success - replace existing files",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireWriteFile(t, filepath.Join(dir, "server.crt"), "old certificate")
				requireWriteFile(t, filepath.Join(dir, "server.key"), "old key")
			},

			assertErr:          require.NoError,
			wantCertificatePEM: "certificate",
			assertKey:          requireKeyFile,
			wantEntries:        []string{"server.crt", "server.key"},
		},
		{
			name: "error - key cannot be replaced",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireWriteFile(t, filepath.Join(dir, "server.crt"), "old certificate")
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "server.key", "dir"), 0o700))
			},

			assertErr:          require.Error,
			wantCertificatePEM: "certificate",
			assertKey: func(t *testing.T, keyFile string) {
				t.Helper()
				_, err := os.ReadFile(keyFile)
				require.Error(t, err, "the directory in place of the key must not be replaced")
			},
			wantEntries: []string{"server.crt", "server.key"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			dir := t.TempDir()
			tc.setup(t, dir)
			repo := NewCertificate(fsroot.Open(t, dir))

			// Run test
			err := repo.UpdateServerKeyPair(t.Context(), []byte("certificate"), []byte("key"))

			// Assert
			tc.assertErr(t, err)

			certificatePEM, err := os.ReadFile(filepath.Join(dir, "server.crt"))
			require.NoError(t, err)
			require.Equal(t, tc.wantCertificatePEM, string(certificatePEM))

			info, err := os.Stat(filepath.Join(dir, "server.crt"))
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the certificate is only readable by the owner")

			tc.assertKey(t, filepath.Join(dir, "server.key"))

			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			entryNames := make([]string, 0, len(entries))
			for _, entry := range entries {
				entryNames = append(entryNames, entry.Name())
			}

			require.Equal(t, tc.wantEntries, entryNames, "no temporary file must be left")
		})
	}
}

func requireKeyFile(t *testing.T, keyFile string) {
	t.Helper()

	keyPEM, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	require.Equal(t, "key", string(keyPEM))

	info, err := os.Stat(keyFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the key is only readable by the owner")
}

func TestCertificate_ValidateKeyPair(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		validate func(repo certificate, dir string) error
		setup    func(t *testing.T, dir string)

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "server - success",
			validate: func(repo certificate, dir string) error {
				return repo.ValidateServerKeyPair(t.Context(), dir)
			},
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireGenCert(t, dir, "server", false)
			},

			assertErr: require.NoError,
		},
		{
			name: "server - error - mismatching key",
			validate: func(repo certificate, dir string) error {
				return repo.ValidateServerKeyPair(t.Context(), dir)
			},
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireGenCert(t, dir, "server", false)

				other := t.TempDir()
				requireGenCert(t, other, "server", false)

				key, err := os.ReadFile(filepath.Join(other, "server.key"))
				require.NoError(t, err)
				requireWriteFile(t, filepath.Join(dir, "server.key"), string(key))
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to validate key pair", a...)
			},
		},
		{
			name: "server - error - key missing",
			validate: func(repo certificate, dir string) error {
				return repo.ValidateServerKeyPair(t.Context(), dir)
			},
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireGenCert(t, dir, "server", false)
				require.NoError(t, os.Remove(filepath.Join(dir, "server.key")))
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, os.ErrNotExist, a...)
			},
		},
		{
			name: "server - error - directory missing",
			validate: func(repo certificate, dir string) error {
				return repo.ValidateServerKeyPair(t.Context(), filepath.Join(dir, "missing"))
			},
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, os.ErrNotExist, a...)
			},
		},
		{
			name: "client - success",
			validate: func(repo certificate, dir string) error {
				return repo.ValidateClientKeyPair(t.Context(), dir)
			},
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireGenCert(t, dir, "client", true)
			},

			assertErr: require.NoError,
		},
		{
			name: "client - error - certificate missing",
			validate: func(repo certificate, dir string) error {
				return repo.ValidateClientKeyPair(t.Context(), dir)
			},
			setup: func(t *testing.T, dir string) {
				t.Helper()
				requireGenCert(t, dir, "client", true)
				require.NoError(t, os.Remove(filepath.Join(dir, "client.crt")))
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, os.ErrNotExist, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			dir := t.TempDir()
			tc.setup(t, dir)

			repo := NewCertificate(fsroot.Open(t, t.TempDir()))

			// Run test
			err := tc.validate(repo, dir)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func requireGenCert(t *testing.T, dir string, name string, isClient bool) {
	t.Helper()

	err := incustls.FindOrGenCert(filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key"), isClient, false)
	require.NoError(t, err)
}
