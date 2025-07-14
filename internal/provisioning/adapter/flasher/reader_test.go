package flasher

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parentCloser(t *testing.T) {
	rc := &closer{Reader: bytes.NewBufferString(`foobar`)}
	parent := &closer{}

	pc := newParentCloser(rc, parent)
	body, err := io.ReadAll(pc)
	require.NoError(t, err)
	require.Equal(t, `foobar`, string(body))

	err = pc.Close()

	require.NoError(t, err)

	require.True(t, rc.closed)
	require.True(t, parent.closed)
}

type closer struct {
	io.Reader
	closed bool
}

func (c *closer) Close() error {
	c.closed = true
	return nil
}

func Test_injectReader(t *testing.T) {
	tests := []struct {
		name      string
		injectPos int

		want string
	}{
		{
			name:      "inject start without head",
			injectPos: 0,

			want: `AAAbarbaz`,
		},
		{
			name:      "inject middle",
			injectPos: 3,

			want: `fooAAAbaz`,
		},
		{
			name:      "inject end without remainder",
			injectPos: 6,

			want: `foobarAAA`,
		},
		{
			name:      "inject end - over length",
			injectPos: 8,

			want: `foobarbaAAA`,
		},
		{
			name:      "append",
			injectPos: 9,

			want: `foobarbazAAA`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := bytes.NewBufferString(`foobarbaz`)
			payload := []byte(`AAA`)

			ir := newInjectReader(io.NopCloser(buf), tc.injectPos, payload)

			got, err := io.ReadAll(ir)
			require.NoError(t, err)

			require.Equal(t, tc.want, string(got))
		})
	}
}
