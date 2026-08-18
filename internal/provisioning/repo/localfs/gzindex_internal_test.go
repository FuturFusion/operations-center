package localfs

import (
	"bytes"
	"compress/gzip"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
)

func writeSeekableTestUpdate(t *testing.T, size int) (string, provisioning.Update, []byte) {
	t.Helper()

	originalInterval := gzIndexInterval
	gzIndexInterval = 1 << 20

	t.Cleanup(func() {
		gzIndexInterval = originalInterval
	})

	storageDir := t.TempDir()

	update := provisioning.Update{
		UUID: uuidgen.FromPattern(t, "1"),
	}

	// Semi-compressible content, so that the compressed file is meaningfully
	// smaller than the original but the deflate stream still holds many blocks.
	contents := make([]byte, size)
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < len(contents); i += 4096 {
		rnd.Read(contents[i:min(i+512, len(contents))])
	}

	var compressed bytes.Buffer

	gzw := gzip.NewWriter(&compressed)
	_, err := gzw.Write(contents)
	require.NoError(t, err)
	require.NoError(t, gzw.Close())

	require.NoError(t, os.MkdirAll(filepath.Join(storageDir, update.UUID.String()), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(storageDir, update.UUID.String(), "image.iso.gz"), compressed.Bytes(), 0o600))

	return storageDir, update, contents
}

func TestLocalfs_GetSeekable(t *testing.T) {
	const size = 8 << 20

	storageDir, update, contents := writeSeekableTestUpdate(t, size)

	repo := localfs{storageDir: storageDir}

	tests := []struct {
		name   string
		offset int64
		length int
	}{
		{name: "from start", offset: 0, length: 4096},
		{name: "from an arbitrary offset", offset: 3_000_001, length: 4096},
		{name: "across an index point", offset: (1 << 20) - 16, length: 4096},
		{name: "to the very end", offset: size - 10, length: 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rsc, gotSize, modTime, err := repo.GetSeekableGZip(t.Context(), update, "image.iso.gz")
			require.NoError(t, err)

			defer func() { _ = rsc.Close() }()

			require.Equal(t, int64(size), gotSize)
			require.False(t, modTime.IsZero())

			_, err = rsc.Seek(tc.offset, io.SeekStart)
			require.NoError(t, err)

			got := make([]byte, tc.length)
			_, err = io.ReadFull(rsc, got)
			require.NoError(t, err)

			require.Equal(t, contents[tc.offset:tc.offset+int64(tc.length)], got)
		})
	}
}

func TestLocalfs_GetSeekableSidecar(t *testing.T) {
	const size = 8 << 20

	storageDir, update, contents := writeSeekableTestUpdate(t, size)

	repo := localfs{storageDir: storageDir}
	sidecar := filepath.Join(storageDir, update.UUID.String(), "image.iso.gz"+gzIndexExt)

	require.NoFileExists(t, sidecar)

	rsc, _, _, err := repo.GetSeekableGZip(t.Context(), update, "image.iso.gz")
	require.NoError(t, err)

	got, err := io.ReadAll(rsc)
	require.NoError(t, err)
	require.Equal(t, contents, got)
	require.NoError(t, rsc.Close())

	require.FileExists(t, sidecar)

	cached := loadGzIndex(t.Context(), filepath.Join(storageDir, update.UUID.String(), "image.iso.gz"))
	require.Equal(t, int64(size), cached.Size)
	require.Greater(t, len(cached.Index), 1)

	rsc, _, _, err = repo.GetSeekableGZip(t.Context(), update, "image.iso.gz")
	require.NoError(t, err)

	defer func() { _ = rsc.Close() }()

	const offset = (1 << 20) + 1234

	_, err = rsc.Seek(offset, io.SeekStart)
	require.NoError(t, err)

	section := make([]byte, 4096)
	_, err = io.ReadFull(rsc, section)
	require.NoError(t, err)

	require.Equal(t, contents[offset:offset+len(section)], section)

	_, err = io.Copy(io.Discard, rsc)
	require.NoError(t, err)
}

func TestLocalfs_GetSeekableSeekEnd(t *testing.T) {
	const size = 1 << 20

	storageDir, update, contents := writeSeekableTestUpdate(t, size)

	repo := localfs{storageDir: storageDir}

	rsc, _, _, err := repo.GetSeekableGZip(t.Context(), update, "image.iso.gz")
	require.NoError(t, err)

	defer func() { _ = rsc.Close() }()

	pos, err := rsc.Seek(-16, io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(size-16), pos)

	got, err := io.ReadAll(rsc)
	require.NoError(t, err)
	require.Equal(t, contents[size-16:], got)
}

func TestLocalfs_PruneFilesKeepsIndexSidecar(t *testing.T) {
	storageDir, update, _ := writeSeekableTestUpdate(t, 1<<20)

	update.Files = provisioning.UpdateFiles{
		{Filename: "image.iso.gz"},
	}

	repo := localfs{storageDir: storageDir}

	rsc, _, _, err := repo.GetSeekableGZip(t.Context(), update, "image.iso.gz")
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, rsc)
	require.NoError(t, err)
	require.NoError(t, rsc.Close())

	sidecar := filepath.Join(storageDir, update.UUID.String(), "image.iso.gz"+gzIndexExt)
	require.FileExists(t, sidecar)

	// An unrelated file is pruned, the sidecar of a file of the update is not.
	stray := filepath.Join(storageDir, update.UUID.String(), "stray.txt")
	require.NoError(t, os.WriteFile(stray, []byte("stray"), 0o600))

	require.NoError(t, repo.PruneFiles(t.Context(), update))

	require.FileExists(t, sidecar)
	require.NoFileExists(t, stray)
}
