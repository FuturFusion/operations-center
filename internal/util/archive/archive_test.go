package archive_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/archive"
)

func TestXZ_roundtrip(t *testing.T) {
	const content = "foobar"

	buf := bytes.NewBuffer(nil)

	w, err := archive.Writer(t.Context(), buf, archive.Packer{"xz"})
	require.NoError(t, err)

	_, err = fmt.Fprint(w, content)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	r, cancel, err := archive.Reader(t.Context(), buf, archive.Unpacker{"xz", "-d"})
	require.NoError(t, err)

	result := bytes.NewBuffer(nil)
	_, err = io.Copy(result, r)
	require.NoError(t, err)
	cancel()

	require.NotEqual(t, buf.Len(), len(content), "compressed content is not expected to have the same length as uncompressed content")
	require.Equal(t, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}, buf.Bytes()[0:6], "xz magic header not found")
	require.Equal(t, content, result.String())
}
