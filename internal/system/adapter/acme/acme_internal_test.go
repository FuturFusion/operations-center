package acme

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func Test_cleanupAccounts(t *testing.T) {
	t.Parallel()

	// Setup
	parentDir := t.TempDir()
	outsideFile := filepath.Join(parentDir, "outside")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside"), 0o600))

	dir := filepath.Join(parentDir, "acme")
	for _, name := range []string{
		"certificates/domain.crt",
		"accounts/acme.example.org/admin@example.org/account.json",
		"accounts/acme.example.org/old@example.org/account.json",
		"accounts/old.example.org/admin@example.org/account.json",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}

	// A symlink, which points outside of the acme directory.
	require.NoError(t, os.Symlink(parentDir, filepath.Join(dir, "accounts", "link")))

	// Run test
	cleanupAccounts(fsroot.Open(t, dir), "acme.example.org", "admin@example.org")

	// Assert
	var gotFiles []string
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			rel, err := filepath.Rel(dir, name)
			require.NoError(t, err)
			gotFiles = append(gotFiles, rel)
		}

		return nil
	})
	require.NoError(t, err)

	require.Equal(t, []string{"accounts/acme.example.org/admin@example.org/account.json"}, gotFiles, "only the account of the current CA and email is kept")
	require.FileExists(t, outsideFile, "files outside of the acme directory must not be removed")
}
