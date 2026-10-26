package log_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/log"
)

func TestSyncBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(buf *log.SyncBuffer) (int, error)

		wantWritten int
		want        string
	}{
		{
			name: "empty",
			run: func(buf *log.SyncBuffer) (int, error) {
				return 0, nil
			},

			wantWritten: 0,
			want:        "",
		},
		{
			name: "multiple writes",
			run: func(buf *log.SyncBuffer) (int, error) {
				n1, err1 := buf.Write([]byte("foo "))
				n2, err2 := buf.Write([]byte("bar"))

				return n1 + n2, errors.Join(err1, err2)
			},

			wantWritten: 7,
			want:        "foo bar",
		},
		{
			name: "reset",
			run: func(buf *log.SyncBuffer) (int, error) {
				n, err := buf.Write([]byte("foo"))

				buf.Reset()

				return n, err
			},

			wantWritten: 3,
			want:        "",
		},
		{
			name: "write after reset",
			run: func(buf *log.SyncBuffer) (int, error) {
				n1, err1 := buf.Write([]byte("foo"))

				buf.Reset()

				n2, err2 := buf.Write([]byte("bar"))

				return n1 + n2, errors.Join(err1, err2)
			},

			wantWritten: 6,
			want:        "bar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			buf := &log.SyncBuffer{}

			// Run test
			n, err := tc.run(buf)

			// Assert
			require.NoError(t, err, "writing to the buffer never fails")
			require.Equal(t, tc.wantWritten, n, "number of written bytes")
			require.Equal(t, tc.want, buf.String(), "content of the buffer")
		})
	}
}

func TestSyncBuffer_concurrent(t *testing.T) {
	t.Parallel()

	// Setup
	const goroutines = 10
	const writes = 100

	buf := &log.SyncBuffer{}
	wg := sync.WaitGroup{}

	// Run test
	for range goroutines {
		wg.Go(func() {
			for range writes {
				_, _ = buf.Write([]byte("x"))
				_ = buf.String()
			}
		})
	}

	wg.Wait()

	// Assert
	require.Equal(t, strings.Repeat("x", goroutines*writes), buf.String(), "all writes are in the buffer")
}
