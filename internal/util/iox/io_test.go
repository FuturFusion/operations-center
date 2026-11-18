package iox_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/iox"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestSafeCopy(t *testing.T) {
	t.Parallel()

	const chunkSize = 4 * 1024 * 1024

	tests := []struct {
		name string
		src  func() io.Reader
		dst  func() io.Writer

		assertErr   require.ErrorAssertionFunc
		wantWritten int64
	}{
		{
			name: "empty source",
			src:  func() io.Reader { return strings.NewReader("") },
			dst:  func() io.Writer { return &bytes.Buffer{} },

			assertErr:   require.NoError,
			wantWritten: 0,
		},
		{
			name: "smaller than one chunk",
			src:  func() io.Reader { return strings.NewReader("content") },
			dst:  func() io.Writer { return &bytes.Buffer{} },

			assertErr:   require.NoError,
			wantWritten: 7,
		},
		{
			name: "exactly one chunk",
			src:  func() io.Reader { return bytes.NewReader(make([]byte, chunkSize)) },
			dst:  func() io.Writer { return &bytes.Buffer{} },

			assertErr:   require.NoError,
			wantWritten: chunkSize,
		},
		{
			name: "multiple chunks",
			src:  func() io.Reader { return bytes.NewReader(make([]byte, 2*chunkSize+1)) },
			dst:  func() io.Writer { return &bytes.Buffer{} },

			assertErr:   require.NoError,
			wantWritten: 2*chunkSize + 1,
		},
		{
			name: "error - write fails",
			src:  func() io.Reader { return strings.NewReader("content") },
			dst:  func() io.Writer { return failingWriter{} },

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "write failed", a...)
			},
			wantWritten: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			src := tc.src()
			dst := tc.dst()

			// Run test
			written, err := iox.SafeCopy(dst, src)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantWritten, written)
		})
	}
}

type closeRecorder struct {
	closed bool
	err    error
}

func (c *closeRecorder) Close() error {
	c.closed = true

	return c.err
}

func TestNewReadCloser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		closeErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:     "close succeeds",
			closeErr: nil,

			assertErr: require.NoError,
		},
		{
			name:     "error - close fails",
			closeErr: errors.New("close failed"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "close failed", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			closer := &closeRecorder{err: tc.closeErr}
			rc := iox.NewReadCloser(strings.NewReader("content"), closer)

			// Run test
			got, readErr := io.ReadAll(rc)
			closeErr := rc.Close()

			// Assert
			require.NoError(t, readErr)
			require.Equal(t, "content", string(got))
			tc.assertErr(t, closeErr)
			require.True(t, closer.closed, "Close must close the given closer")
		})
	}
}
