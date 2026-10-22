package log_test

import (
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
		run  func(t *testing.T, buf *log.SyncBuffer)

		want string
	}{
		{
			name: "empty",
			run: func(t *testing.T, buf *log.SyncBuffer) {
				t.Helper()
			},

			want: "",
		},
		{
			name: "multiple writes",
			run: func(t *testing.T, buf *log.SyncBuffer) {
				t.Helper()

				n, err := buf.Write([]byte("foo "))
				require.NoError(t, err)
				require.Equal(t, 4, n, "number of written bytes")

				_, err = buf.Write([]byte("bar"))
				require.NoError(t, err)
			},

			want: "foo bar",
		},
		{
			name: "reset",
			run: func(t *testing.T, buf *log.SyncBuffer) {
				t.Helper()

				_, err := buf.Write([]byte("foo"))
				require.NoError(t, err)

				buf.Reset()
			},

			want: "",
		},
		{
			name: "write after reset",
			run: func(t *testing.T, buf *log.SyncBuffer) {
				t.Helper()

				_, err := buf.Write([]byte("foo"))
				require.NoError(t, err)

				buf.Reset()

				_, err = buf.Write([]byte("bar"))
				require.NoError(t, err)
			},

			want: "bar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			buf := &log.SyncBuffer{}

			// Run test
			tc.run(t, buf)

			// Assert
			require.Equal(t, tc.want, buf.String())
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

	buf.Reset()
	require.Empty(t, buf.String(), "buffer is empty after reset")
}
