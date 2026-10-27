package localfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestLocalfs_CleanupAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		namespaces  []string
		setupTmpDir func(t *testing.T, destDir string)

		assertErr    require.ErrorAssertionFunc
		assertTmpDir func(t *testing.T, destDir string)
	}{
		{
			name:       "success - empty",
			namespaces: []string{"seed-images"},
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(destDir, "seed-images"), 0o700)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				requireDirEmpty(t, filepath.Join(destDir, "seed-images"))
			},
		},
		{
			name:       "success - namespace does not exist",
			namespaces: []string{"seed-images"},
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assertTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				require.NoDirExists(t, filepath.Join(destDir, "seed-images"))
			},
		},
		{
			name:       "success - with content",
			namespaces: []string{"seed-images"},
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(destDir, "seed-images", "public"), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, "seed-images", "public", "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, "seed-images", "file2.txt"), []byte(`file2 body`), 0o600)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				requireDirEmpty(t, filepath.Join(destDir, "seed-images"))
			},
		},
		{
			name:       "success - content outside of the namespaces is kept",
			namespaces: []string{"seed-images"},
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(destDir, "seed-images"), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, "seed-images", "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)

				// The ACME account key can not be regenerated.
				err = os.MkdirAll(filepath.Join(destDir, "acme", "accounts"), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, "acme", "accounts", "key.pem"), []byte(`account key`), 0o600)
				require.NoError(t, err)

				// The cache dir coincides with the var dir, if OPERATIONS_CENTER_DIR is set.
				err = os.WriteFile(filepath.Join(destDir, "local.db"), []byte(`database`), 0o600)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				requireDirEmpty(t, filepath.Join(destDir, "seed-images"))
				require.FileExists(t, filepath.Join(destDir, "acme", "accounts", "key.pem"))
				require.FileExists(t, filepath.Join(destDir, "local.db"))
			},
		},
		{
			name:       "error - namespace outside of the cache directory",
			namespaces: []string{"../var"},
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				err := os.WriteFile(filepath.Join(destDir, "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)
			},

			assertErr: errassert.Contains(`Invalid cache namespace "../var"`),
			assertTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				require.FileExists(t, filepath.Join(destDir, "file1.txt"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs := New(fsroot.Open(t, tmpDir), tc.namespaces...)

			// Run test
			err := lfs.CleanupAll(t.Context())

			// Assert
			tc.assertErr(t, err)
			tc.assertTmpDir(t, tmpDir)
		})
	}
}

func requireDirEmpty(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
