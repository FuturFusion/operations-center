package metrics_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestRetryableLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error

		want string
	}{
		{
			name: "plain error",
			err:  boom.Error,

			want: "false",
		},
		{
			name: "retryable error",
			err:  domain.NewRetryableErr(boom.Error),

			want: "true",
		},
		{
			name: "wrapped retryable error",
			err:  fmt.Errorf("outer: %w", domain.NewRetryableErr(boom.Error)),

			want: "true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			got := metrics.RetryableLabel(tc.err)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
