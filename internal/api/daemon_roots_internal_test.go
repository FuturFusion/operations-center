package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestDaemon_openRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		varDir   string
		cacheDir string

		assertErr require.ErrorAssertionFunc
		wantDirs  []string
	}{
		{
			name:     "success - missing directories are created",
			varDir:   "var",
			cacheDir: "cache/operations-center",

			assertErr: require.NoError,
			wantDirs:  []string{"var", "cache/operations-center"},
		},
		{
			name:     "success - var and cache directory are the same",
			varDir:   "var",
			cacheDir: "var",

			assertErr: require.NoError,
			wantDirs:  []string{"var"},
		},
		{
			name:     "error - var directory path component is a file",
			varDir:   "file/var",
			cacheDir: "cache",

			assertErr: require.Error,
			wantDirs:  []string{},
		},
		{
			name:     "error - cache directory path component is a file",
			varDir:   "var",
			cacheDir: "file/cache",

			assertErr: require.Error,
			wantDirs:  []string{"var"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			err := os.WriteFile(filepath.Join(tmpDir, "file"), nil, 0o600)
			require.NoError(t, err)

			d := &Daemon{
				env: &mock.EnvironmentMock{
					VarDirFunc: func() string {
						return filepath.Join(tmpDir, tc.varDir)
					},
					CacheDirFunc: func() string {
						return filepath.Join(tmpDir, tc.cacheDir)
					},
				},
			}

			// Run test
			err = d.openRoots()

			// Assert
			tc.assertErr(t, err)
			for _, dir := range tc.wantDirs {
				require.DirExists(t, filepath.Join(tmpDir, dir))
			}

			require.NoError(t, d.closeRoots(), "roots must close without error, even if opening failed")
		})
	}
}

func TestDaemon_loadClientCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)

		wantCertificate string
		wantKey         string
	}{
		{
			name: "success",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(dir, "client.crt"), []byte("certificate"), 0o600)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dir, "client.key"), []byte("key"), 0o600)
				require.NoError(t, err)
			},

			wantCertificate: "certificate",
			wantKey:         "key",
		},
		{
			name: "files missing",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},

			wantCertificate: "",
			wantKey:         "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setup(t, tmpDir)

			d := &Daemon{
				varRoot: fsroot.Open(t, tmpDir),
			}

			// Run test
			d.loadClientCertificate(t.Context())

			// Assert
			require.Equal(t, tc.wantCertificate, d.clientCertificate)
			require.Equal(t, tc.wantKey, d.clientKey)
		})
	}
}
