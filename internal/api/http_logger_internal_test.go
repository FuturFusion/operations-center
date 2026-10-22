package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeadlineFrom(t *testing.T) {
	t.Parallel()

	withTimeout := func(timeout time.Duration) func(t *testing.T) context.Context {
		return func(t *testing.T) context.Context {
			t.Helper()

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			t.Cleanup(cancel)

			return ctx
		}
	}

	tests := []struct {
		name string
		ctx  func(t *testing.T) context.Context

		want time.Duration
	}{
		{
			name: "without deadline",
			ctx: func(t *testing.T) context.Context {
				t.Helper()

				return context.Background()
			},

			want: 60 * time.Second,
		},
		{
			name: "deadline in the future",
			ctx:  withTimeout(time.Hour),

			want: time.Hour,
		},
		{
			name: "deadline closer than the grace period",
			ctx:  withTimeout(time.Millisecond),

			want: shutdownGracePeriod,
		},
		{
			name: "deadline passed",
			ctx:  withTimeout(-time.Second),

			want: shutdownGracePeriod,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := tc.ctx(t)

			got := deadlineFrom(ctx, 60*time.Second)

			require.InDelta(t, tc.want, got, float64(time.Second), "expect the time to stop the task to be %v, got %v", tc.want, got)
		})
	}
}
