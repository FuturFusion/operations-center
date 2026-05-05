package errassert

import (
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func Contains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorContains(tt, err, contains, a...)
	}
}

func OperationNotPermittedError(tt require.TestingT, err error, a ...any) {
	require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
}

func OperationNotPermittedErrorContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
		require.ErrorContains(tt, err, contains, a...)
	}
}

func RetryableBoomError(tt require.TestingT, err error, a ...any) {
	boom.ErrorIs(tt, err, a...)
	var retryableErr domain.ErrRetryable
	require.ErrorAs(tt, err, &retryableErr, a...)
}

func RetryableError(tt require.TestingT, err error, a ...any) {
	var retryableErr domain.ErrRetryable
	require.ErrorAs(tt, err, &retryableErr, a...)
}

func RetryableErrorContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		var retryableErr domain.ErrRetryable
		require.ErrorAs(tt, err, &retryableErr, a...)
		require.ErrorContains(tt, err, contains, a...)
	}
}

func TerminalError(tt require.TestingT, err error, a ...any) {
	require.ErrorIs(tt, err, domain.ErrTerminal, a...)
}

func TerminalErrorContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorIs(tt, err, domain.ErrTerminal, a...)
		require.ErrorContains(tt, err, contains, a...)
	}
}

func ValidationError(tt require.TestingT, err error, a ...any) {
	var verr domain.ErrValidation
	require.ErrorAs(tt, err, &verr, a...)
}

func ValidationErrorContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		var verr domain.ErrValidation
		require.ErrorAs(tt, err, &verr, a...)
		require.ErrorContains(tt, err, contains, a...)
	}
}
