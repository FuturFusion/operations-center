package flasher

import (
	"bytes"
	"errors"
	"io"
	"strings"
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

func Test_injectSeeker(t *testing.T) {
	const source = `0123456789abcdefghij`

	payload := []byte(`XYZ`)

	// The seeded stream the seeker has to reproduce from any offset.
	want := []byte(source)
	copy(want[5:8], payload)

	tests := []struct {
		name   string
		offset int64
		whence int

		wantPos int64
	}{
		{name: "start of stream", offset: 0, whence: io.SeekStart, wantPos: 0},
		{name: "before payload", offset: 4, whence: io.SeekStart, wantPos: 4},
		{name: "first byte of payload", offset: 5, whence: io.SeekStart, wantPos: 5},
		{name: "middle of payload", offset: 6, whence: io.SeekStart, wantPos: 6},
		{name: "first byte after payload", offset: 8, whence: io.SeekStart, wantPos: 8},
		{name: "past payload", offset: 12, whence: io.SeekStart, wantPos: 12},
		{name: "relative to end", offset: -4, whence: io.SeekEnd, wantPos: 16},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			is := newInjectSeeker(nopReadSeekCloser{strings.NewReader(source)}, 5, payload)

			pos, err := is.Seek(tc.offset, tc.whence)
			require.NoError(t, err)
			require.Equal(t, tc.wantPos, pos)

			got, err := io.ReadAll(is)
			require.NoError(t, err)

			require.Equal(t, string(want[tc.wantPos:]), string(got))
		})
	}
}

func Test_injectSeekerSeekBackAndForth(t *testing.T) {
	const source = `0123456789abcdefghij`

	payload := []byte(`XYZ`)

	want := []byte(source)
	copy(want[5:8], payload)

	is := newInjectSeeker(nopReadSeekCloser{strings.NewReader(source)}, 5, payload)

	// Read a section overlapping the payload, jump backwards over it and read
	// the very same section again.
	for range 2 {
		_, err := is.Seek(6, io.SeekStart)
		require.NoError(t, err)

		got := make([]byte, 4)
		_, err = io.ReadFull(is, got)
		require.NoError(t, err)

		require.Equal(t, string(want[6:10]), string(got))
	}
}

type nopReadSeekCloser struct {
	io.ReadSeeker
}

func (nopReadSeekCloser) Close() error { return nil }
