package errassert

import (
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Contains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorContains(tt, err, contains, a...)
	}
}

func NotFoundError(tt require.TestingT, err error, a ...any) {
	require.ErrorIs(tt, err, domain.ErrNotFound, a...)
}

func NotFoundErrorContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorIs(tt, err, domain.ErrNotFound, a...)
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

// DomainError asserts that err is a domain.Error of the given kind, which
// reports the given reason.
func DomainError(kind error, reason api.ErrorReason) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorIs(tt, err, kind, a...)
		ReasonIs(reason)(tt, err, a...)
	}
}

// ReasonIs asserts that err reports the given reason.
func ReasonIs(reason api.ErrorReason) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		var domainErr *domain.Error
		require.ErrorAs(tt, err, &domainErr, a...)
		require.Equal(tt, string(reason), domainErr.Reason(), a...)
	}
}

// UserMessageContains asserts that the message of err, which is reported to the
// user, contains the given string.
func UserMessageContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.Error(tt, err, a...)
		require.Contains(tt, domain.UserMessage(err), contains, a...)
	}
}
