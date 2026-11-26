package metrics_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error

		want bool
	}{
		{
			name: "nil error",
			err:  nil,

			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("boom"),

			want: false,
		},
		{
			name: "retryable error",
			err:  domain.NewRetryableErr(errors.New("boom")),

			want: true,
		},
		{
			name: "wrapped retryable error",
			err:  fmt.Errorf("outer: %w", domain.NewRetryableErr(errors.New("boom"))),

			want: true,
		},
		{
			name: "plain error wrapped by retryable error",
			err:  domain.NewRetryableErr(fmt.Errorf("outer: %w", errors.New("boom"))),

			want: true,
		},
		{
			name: "joined with retryable error",
			err:  errors.Join(errors.New("boom"), domain.NewRetryableErr(errors.New("boom"))),

			want: false,
		},
		{
			name: "joined retryable errors",
			err:  errors.Join(domain.NewRetryableErr(errors.New("boom")), domain.NewRetryableErr(errors.New("boom"))),

			want: true,
		},
		{
			name: "wrapped join with retryable error",
			err:  fmt.Errorf("outer: %w", errors.Join(errors.New("boom"), domain.NewRetryableErr(errors.New("boom")))),

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := metrics.IsRetryable(tc.err)

			require.Equal(t, tc.want, got, "a joined error is retryable only if all its errors are retryable")
		})
	}
}
