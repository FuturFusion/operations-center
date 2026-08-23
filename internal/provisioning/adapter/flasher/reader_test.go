package flasher

import (
	"bytes"
	"errors"
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
		injectPos int64
		readSize  int

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

			want: `foobarbaA`,
		},
		{
			name:      "inject after end",
			injectPos: 9,

			want: `foobarbaz`,
		},
		{
			name:      "inject middle - single byte reads",
			injectPos: 3,
			readSize:  1,

			want: `fooAAAbaz`,
		},
		{
			name:      "inject middle - reads straddling both payload bounds",
			injectPos: 3,
			readSize:  4,

			want: `fooAAAbaz`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := bytes.NewBufferString(`foobarbaz`)
			payload := []byte(`AAA`)

			ir := newInjectReader(io.NopCloser(buf), tc.injectPos, payload)

			var got []byte
			var err error

			if tc.readSize == 0 {
				got, err = io.ReadAll(ir)
			} else {
				got, err = readInChunks(ir, tc.readSize)
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}

func readInChunks(r io.Reader, chunkSize int) ([]byte, error) {
	var got []byte

	for {
		buf := make([]byte, chunkSize)

		n, err := r.Read(buf)
		got = append(got, buf[:n]...)

		if errors.Is(err, io.EOF) {
			return got, nil
		}

		if err != nil {
			return got, err
		}
	}
}
