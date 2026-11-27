package memberfiles_test

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestLocal(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	varDir := filepath.Join(parentDir, "var")
	require.NoError(t, os.MkdirAll(filepath.Join(varDir, memberfiles.StoreUpdates, "u1", "x86_64"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(varDir, memberfiles.StoreUpdates, "u1", "x86_64", "a.raw"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(parentDir, "outside"), []byte("outside"), 0o600))

	local := memberfiles.NewLocal(fsroot.Open(t, varDir))

	f, err := local.Open(memberfiles.StoreUpdates, "u1/x86_64/a.raw")
	require.NoError(t, err)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.Equal(t, "a", string(content))

	buf := &bytes.Buffer{}
	require.NoError(t, local.WriteArchive(buf, memberfiles.StoreUpdates, "u1"))
	header, err := tar.NewReader(buf).Next()
	require.NoError(t, err)
	require.Equal(t, "x86_64/a.raw", header.Name)

	// Names, which would leave the store, are refused.
	for _, name := range []string{"", "..", "../outside", "../../outside", "/etc/passwd"} {
		_, err = local.Stat(memberfiles.StoreUpdates, name)
		require.Error(t, err, name)

		require.Error(t, local.Delete(memberfiles.StoreUpdates, name+"/x"), name)
	}

	_, err = local.Open("bogus", "u1")
	require.Error(t, err)

	require.NoError(t, local.Delete(memberfiles.StoreUpdates, ""))
	require.NoDirExists(t, filepath.Join(varDir, memberfiles.StoreUpdates, "u1"))
	require.DirExists(t, filepath.Join(varDir, memberfiles.StoreUpdates))
	require.FileExists(t, filepath.Join(parentDir, "outside"))
}
