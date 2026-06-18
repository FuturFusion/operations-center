package xz_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/archive/xz"
)

func TestXZ_roundtrip(t *testing.T) {
	const content = "foobar"

	buf := bytes.NewBuffer(nil)

	xzWriter, err := xz.NewWriter(t.Context(), buf)
	require.NoError(t, err)

	_, err = fmt.Fprint(xzWriter, content)
	require.NoError(t, err)
	err = xzWriter.Close()
	require.NoError(t, err)

	xzReader, err := xz.NewReader(t.Context(), buf)
	require.NoError(t, err)

	result := bytes.NewBuffer(nil)
	_, err = io.Copy(result, xzReader)
	require.NoError(t, err)
	err = xzReader.Close()
	require.NoError(t, err)

	require.NotEqual(t, buf.Len(), len(content), "compressed content is not expected to have the same length as uncompressed content")
	require.Equal(t, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}, buf.Bytes()[0:6], "xz magic header not found")
	require.Equal(t, content, result.String())
}
