package flasher

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_uncompressedSize(t *testing.T) {
	content := strings.Repeat("incus-os", 128)

	gzipped := func(t *testing.T) *bytes.Reader {
		t.Helper()

		var buf bytes.Buffer

		gzipWriter := gzip.NewWriter(&buf)

		_, err := gzipWriter.Write([]byte(content))
		require.NoError(t, err)

		require.NoError(t, gzipWriter.Close())

		return bytes.NewReader(buf.Bytes())
	}

	t.Run("reports the size from the gzip footer", func(t *testing.T) {
		reader := gzipped(t)

		size, err := uncompressedSize(reader, len(content))
		require.NoError(t, err)
		require.Equal(t, len(content), size)
	})

	t.Run("leaves the reader at its position", func(t *testing.T) {
		reader := gzipped(t)
		compressed, err := io.ReadAll(reader)
		require.NoError(t, err)

		_, err = reader.Seek(0, io.SeekStart)
		require.NoError(t, err)

		_, err = uncompressedSize(reader, len(content))
		require.NoError(t, err)

		got, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, compressed, got)
	})

	// ISIZE holds the uncompressed size modulo 2^32, a value below the minimum
	// size means it wrapped around.
	t.Run("reports an implausibly small size as unknown", func(t *testing.T) {
		reader := gzipped(t)

		size, err := uncompressedSize(reader, len(content)+1)
		require.NoError(t, err)
		require.Equal(t, -1, size)
	})

	t.Run("reports the size as unknown for a reader which cannot seek", func(t *testing.T) {
		compressed, err := io.ReadAll(gzipped(t))
		require.NoError(t, err)

		size, err := uncompressedSize(readerOnly{Reader: bytes.NewReader(compressed)}, len(content))
		require.NoError(t, err)
		require.Equal(t, -1, size)
	})

	t.Run("errors out on a stream too short to hold a footer", func(t *testing.T) {
		_, err := uncompressedSize(bytes.NewReader([]byte{0x00, 0x01}), 1)
		require.Error(t, err)
	})

	t.Run("a wrapped around size is only detected below the minimum", func(t *testing.T) {
		// A gzip stream claiming to hold 2 GiB of content, the size the seeded
		// image is expected to have at least.
		var footer [4]byte
		binary.LittleEndian.PutUint32(footer[:], 1<<31)

		size, err := uncompressedSize(bytes.NewReader(footer[:]), 1<<31)
		require.NoError(t, err)
		require.Equal(t, 1<<31, size)
	})
}

type readerOnly struct {
	io.Reader
}
