package file_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "target")

	err := file.WriteFileAtomic(filename, []byte("first"), 0o600)
	require.NoError(t, err)

	contents, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, "first", string(contents))

	fileInfo, err := os.Stat(filename)
	require.NoError(t, err)

	// In contrast to os.WriteFile, the mode is not masked by the umask.
	require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	err = file.WriteFileAtomic(filename, []byte("second"), 0o600)
	require.NoError(t, err)

	contents, err = os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, "second", string(contents))

	// The temporary file is not left behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	err = file.WriteFileAtomic(filepath.Join(dir, "missing", "target"), []byte("nope"), 0o600)
	require.Error(t, err)
}
