package xz_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image/adapter/xz"
)

func TestXZ_roundtrip(t *testing.T) {
	t.Parallel()

	const content = "foobar"

	buf := bytes.NewBuffer(nil)
	adapter := xz.New()

	w, err := adapter.NewWriter(t.Context(), buf)
	require.NoError(t, err)

	_, err = fmt.Fprint(w, content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	require.Equal(t, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}, buf.Bytes()[:6], "xz magic header not found")

	r, err := adapter.NewReader(t.Context(), buf)
	require.NoError(t, err)

	result, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	require.Equal(t, content, string(result))
}
