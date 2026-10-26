package memberfiles_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
)

func TestDeleteUnknownFiles(t *testing.T) {
	dir := t.TempDir()

	old := time.Now().Add(-2 * time.Minute)
	for _, name := range []string{"a/known", "a/unknown", "b/c/unknown", "fresh/unknown"} {
		filename := filepath.Join(dir, filepath.FromSlash(name))

		err := os.MkdirAll(filepath.Dir(filename), 0o700)
		require.NoError(t, err)

		err = os.WriteFile(filename, []byte(name), 0o600)
		require.NoError(t, err)

		if filepath.Dir(name) != "fresh" {
			err = os.Chtimes(filename, old, old)
			require.NoError(t, err)
		}
	}

	err := memberfiles.DeleteUnknownFiles(dir, map[string]bool{"a/known": true})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, "a", "known"))
	require.NoFileExists(t, filepath.Join(dir, "a", "unknown"))
	require.NoDirExists(t, filepath.Join(dir, "b"))
	require.FileExists(t, filepath.Join(dir, "fresh", "unknown"))
	require.DirExists(t, dir)
}

func TestDeleteUnknownFiles_missingDir(t *testing.T) {
	err := memberfiles.DeleteUnknownFiles(filepath.Join(t.TempDir(), "missing"), nil)
	require.NoError(t, err)
}

func TestDeleteStores(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"updates/a/file", "images/b/file", "artifacts/c/file", "database/global"} {
		filename := filepath.Join(dir, filepath.FromSlash(name))

		err := os.MkdirAll(filepath.Dir(filename), 0o700)
		require.NoError(t, err)

		err = os.WriteFile(filename, []byte(name), 0o600)
		require.NoError(t, err)
	}

	err := memberfiles.DeleteStores(dir)
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(dir, "updates", "a"))
	require.NoDirExists(t, filepath.Join(dir, "images", "b"))
	require.NoDirExists(t, filepath.Join(dir, "artifacts", "c"))
	require.FileExists(t, filepath.Join(dir, "database", "global"))
}

func TestLocal_archive(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	files := map[string]string{"artifacts/uuid/one.txt": "one", "artifacts/uuid/sub/two.txt": "two"}
	for name, content := range files {
		writeFile(t, sourceDir, name, content)
	}

	writeFile(t, targetDir, "artifacts/uuid/stale.txt", "stale")

	buf := &bytes.Buffer{}
	err := memberfiles.NewLocal(sourceDir).WriteArchive(buf, memberfiles.StoreArtifacts, "uuid")
	require.NoError(t, err)

	err = memberfiles.NewLocal(targetDir).ExtractArchive(buf, memberfiles.StoreArtifacts, "uuid")
	require.NoError(t, err)

	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(name)))
		require.NoError(t, err)
		require.Equal(t, content, string(got))
	}

	require.NoFileExists(t, filepath.Join(targetDir, "artifacts", "uuid", "stale.txt"), "the directory is replaced")

	err = memberfiles.NewLocal(sourceDir).WriteArchive(buf, memberfiles.StoreArtifacts, "missing")
	require.ErrorIs(t, err, domain.ErrNotFound)

	err = memberfiles.NewLocal(sourceDir).WriteArchive(buf, memberfiles.StoreArtifacts, "../escape")
	require.ErrorAs(t, err, new(domain.ErrValidation))
}

func writeFile(t *testing.T, dir string, name string, content string) {
	t.Helper()

	filename := filepath.Join(dir, filepath.FromSlash(name))

	err := os.MkdirAll(filepath.Dir(filename), 0o700)
	require.NoError(t, err)

	err = os.WriteFile(filename, []byte(content), 0o600)
	require.NoError(t, err)
}
