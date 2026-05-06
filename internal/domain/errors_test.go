package domain_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestValidationErr_Error(t *testing.T) {
	err := domain.NewValidationErrf("boom!")

	require.Equal(t, "boom!", err.Error())
}

func TestRetryableErrf(t *testing.T) {
	err := fmt.Errorf("Outer wrap: %w", domain.NewRetryableErr(boom.Error))

	var retryableErr domain.ErrRetryable
	require.ErrorAs(t, err, &retryableErr)
}

func TestIsRetryableError(t *testing.T) {
	nilRetryableErr := domain.NewRetryableErr(nil)
	require.False(t, domain.IsRetryableError(nilRetryableErr))

	err := boom.Error
	require.False(t, domain.IsRetryableError(err))

	retryableErr := domain.NewRetryableErr(err)
	require.True(t, domain.IsRetryableError(retryableErr))

	unwrappedRetryableErr, ok := retryableErr.(domain.ErrRetryable)
	require.True(t, ok)
	innerBoomErr := unwrappedRetryableErr.Unwrap()
	boom.ErrorIs(t, innerBoomErr)
}

func TestRetryableWrapper(t *testing.T) {
	tests := []struct {
		name  string
		inErr error

		want bool
	}{
		{
			name:  "nil",
			inErr: nil,

			want: false,
		},
		{
			name:  "any error",
			inErr: errors.New("any error"),

			want: false,
		},
		{
			name:  "syscall.ECONNREFUSED",
			inErr: syscall.ECONNREFUSED,

			want: true,
		},
		{
			name:  "io.EOF",
			inErr: io.EOF,

			want: true,
		},
		{
			name:  "wrapped io.EOF",
			inErr: fmt.Errorf("wrapper: %w", io.EOF),

			want: true,
		},
		{
			name:  "io.ErrUnexpectedEOF",
			inErr: io.ErrUnexpectedEOF,

			want: true,
		},
		{
			name:  "context.DeadlineExceeded",
			inErr: context.DeadlineExceeded,

			want: true,
		},
		{
			name:  "context.Canceled",
			inErr: context.Canceled,

			want: true,
		},
		{
			name:  "no available cowsql leader server found",
			inErr: fmt.Errorf("any error: %w", errors.New("no available cowsql leader server found")),

			want: true,
		},
		{
			name:  "context deadline exceeded",
			inErr: fmt.Errorf("any error: %w", errors.New("context deadline exceeded")),

			want: true,
		},
		{
			name:  "Unable to connect to: 127.0.0.1 (context cancelled)",
			inErr: fmt.Errorf("any error: %w", errors.New("Unable to connect to: 127.0.0.1 (context cancelled)")),

			want: true,
		},
		{
			name:  "Unable to connect to: 127.0.0.1 (connection refused)",
			inErr: fmt.Errorf("any error: %w", errors.New("Unable to connect to: 127.0.0.1 (connection refused)")),

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.RetryableWrapper()(tc.inErr)

			require.Equal(t, tc.want, domain.IsRetryableError(err))
		})
	}
}
