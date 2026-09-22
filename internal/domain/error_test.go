package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestError_ErrorAndMessage(t *testing.T) {
	err := domain.NewErrorf(domain.ErrOperationNotPermitted, "server_not_evacuated", "Server %q is not evacuated", "one")

	require.Equal(t, `Server "one" is not evacuated`, err.Error(), "message is reported without the kind")
	require.Equal(t, `Server "one" is not evacuated`, err.Message())

	withCause := err.WithCause(boom.Error)

	require.Equal(t, `Server "one" is not evacuated: boom!`, withCause.Error(), "error message contains the cause for the log")
	require.Equal(t, `Server "one" is not evacuated`, withCause.Message(), "user message does not contain the cause")
	require.Equal(t, `Server "one" is not evacuated`, err.Error(), "the original error is not modified")
}

func TestError_Details(t *testing.T) {
	err := domain.NewErrorf(domain.ErrNotFound, "", "Server %q not found", "one")
	withDetail := err.WithDetail("server", "one")

	require.Empty(t, err.Details(), "the original error is not modified")
	require.Equal(t, map[string]string{"server": "one"}, withDetail.Details())
	require.Empty(t, err.Reason(), "reason is optional")
}

func TestError_Is(t *testing.T) {
	err := fmt.Errorf("Failed to delete server: %w", domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Server is a cluster member"))

	require.ErrorIs(t, err, domain.ErrOperationNotPermitted, "the kind is matched through wrapping")
	require.NotErrorIs(t, err, domain.ErrNotFound)
}

func TestError_As(t *testing.T) {
	err := fmt.Errorf("Failed to update server: %w", domain.NewErrorf(domain.ErrInvalidArgument, "", "Server name can not be empty"))

	var validationErr domain.ErrValidation
	require.ErrorAs(t, err, &validationErr, "an error of kind ErrInvalidArgument is a validation error")

	var domainErr *domain.Error
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, "Server name can not be empty", domainErr.Message())
}

func TestError_Unwrap(t *testing.T) {
	err := domain.NewErrorf(domain.ErrNotFound, "", "Server %q not found", "one").WithCause(boom.Error)

	boom.ErrorIs(t, err, "the cause stays part of the error chain")
}

func TestUserMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error

		want string
	}{
		{
			name: "nil",
			err:  nil,
			want: "",
		},
		{
			name: "domain error - outermost wins",
			err: fmt.Errorf("Failed to remove servers from cluster %q: %w", "one",
				domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Server %q must be evacuated", "member").WithCause(boom.Error),
			),
			want: `Server "member" must be evacuated`,
		},
		{
			name: "wrapped kind - suffix removed",
			err:  fmt.Errorf("Server %q is clustered: %w", "one", domain.ErrOperationNotPermitted),
			want: `Server "one" is clustered`,
		},
		{
			name: "wrapped kind in the middle of the chain",
			err: fmt.Errorf("Failed to delete server: %w",
				fmt.Errorf("Server %q is clustered: %w", "one", domain.ErrOperationNotPermitted),
			),
			want: `Failed to delete server: Server "one" is clustered`,
		},
		{
			name: "bare kind",
			err:  domain.ErrNotFound,
			want: "Not found",
		},
		{
			name: "plain error",
			err:  boom.Error,
			want: "boom!",
		},
		{
			name: "retryable error",
			err:  domain.NewRetryableErr(errors.New("Unable to connect to: 1.2.3.4")),
			want: "Unable to connect to: 1.2.3.4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, domain.UserMessage(tc.err))
		})
	}
}
