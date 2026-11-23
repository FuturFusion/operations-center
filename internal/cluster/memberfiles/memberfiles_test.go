package memberfiles_test

import (
	"archive/tar"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
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

	err := memberfiles.DeleteUnknownFiles(fsroot.Open(t, dir), map[string]bool{"a/known": true})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, "a", "known"))
	require.NoFileExists(t, filepath.Join(dir, "a", "unknown"))
	require.NoDirExists(t, filepath.Join(dir, "b"))
	require.FileExists(t, filepath.Join(dir, "fresh", "unknown"))
	require.DirExists(t, dir)
}

func TestDeleteUnknownFiles_missingDir(t *testing.T) {
	err := memberfiles.DeleteUnknownFilesIn(fsroot.Open(t, t.TempDir()), "missing", nil)
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

	err := memberfiles.DeleteStores(fsroot.Open(t, dir))
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
	err := memberfiles.NewLocal(fsroot.Open(t, sourceDir)).WriteArchive(buf, memberfiles.StoreArtifacts, "uuid")
	require.NoError(t, err)

	err = memberfiles.NewLocal(fsroot.Open(t, targetDir)).ExtractArchive(buf, memberfiles.StoreArtifacts, "uuid")
	require.NoError(t, err)

	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(name)))
		require.NoError(t, err)
		require.Equal(t, content, string(got))
	}

	require.NoFileExists(t, filepath.Join(targetDir, "artifacts", "uuid", "stale.txt"), "the directory is replaced")

	err = memberfiles.NewLocal(fsroot.Open(t, sourceDir)).WriteArchive(buf, memberfiles.StoreArtifacts, "missing")
	require.ErrorIs(t, err, domain.ErrNotFound)

	err = memberfiles.NewLocal(fsroot.Open(t, sourceDir)).WriteArchive(buf, memberfiles.StoreArtifacts, "../escape")
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

func TestLocal_ExtractArchive_outsideTarget(t *testing.T) {
	tests := []struct {
		name    string
		entries []*tar.Header
	}{
		{
			name:    "parent directory",
			entries: []*tar.Header{{Name: "../../escape.txt"}},
		},
		{
			name:    "absolute path",
			entries: []*tar.Header{{Name: "/escape.txt"}},
		},
		{
			name: "through a symbolic link",
			entries: []*tar.Header{
				{Name: "link", Typeflag: tar.TypeSymlink, Linkname: ".."},
				{Name: "link/escape.txt"},
				{Name: "../escape.txt"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			targetDir := filepath.Join(root, "var")
			writeFile(t, targetDir, "artifacts/uuid/kept.txt", "kept")

			buf := &bytes.Buffer{}
			tw := tar.NewWriter(buf)
			for _, header := range tc.entries {
				if header.Typeflag == 0 {
					header.Typeflag = tar.TypeReg
					header.Size = int64(len("escape"))
				}

				header.Mode = 0o600
				require.NoError(t, tw.WriteHeader(header))

				if header.Typeflag == tar.TypeReg {
					_, err := tw.Write([]byte("escape"))
					require.NoError(t, err)
				}
			}

			require.NoError(t, tw.Close())

			err := memberfiles.NewLocal(fsroot.Open(t, targetDir)).ExtractArchive(buf, memberfiles.StoreArtifacts, "uuid")
			require.ErrorAs(t, err, new(domain.ErrValidation))

			// Nothing is written outside the directory, and the directory and its files are kept.
			var written []string
			err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err == nil && !entry.IsDir() {
					written = append(written, path)
				}

				return err
			})
			require.NoError(t, err)
			require.Equal(t, []string{filepath.Join(targetDir, "artifacts", "uuid", "kept.txt")}, written)
		})
	}
}
