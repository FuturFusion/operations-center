package domain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"syscall"
)

var (
	ErrNotFound = errors.New("Not found")

	ErrConstraintViolation = errors.New("Constraint violation")

	ErrOperationNotPermitted = errors.New("Operation not permitted")

	ErrNotAuthenticated = errors.New("Not authenticated")

	ErrNotAuthorized = errors.New("Not authorized")

	ErrTerminal = errors.New("Terminal")

	ErrInvalidArgument = ErrValidation("Invalid argument")
)

type ErrValidation string

func NewValidationErrf(format string, a ...any) error {
	return ErrValidation(fmt.Sprintf(format, a...))
}

func (e ErrValidation) Error() string {
	return string(e)
}

type ErrRetryable struct {
	innerErr error
}

// NewRetryableErr wraps the provided error as a ErrRetryable, if the
// passed err is none nil. If the passed err is nil, this function does
// not wrap and returns nil.
func NewRetryableErr(err error) error {
	if err == nil {
		return nil
	}

	return ErrRetryable{
		innerErr: err,
	}
}

func (e ErrRetryable) Error() string {
	return e.innerErr.Error()
}

func (e ErrRetryable) Unwrap() error {
	return e.innerErr
}

func IsRetryableError(err error) bool {
	var retryableErr ErrRetryable
	return errors.As(err, &retryableErr)
}

// ErrNotSettled is a request, that was turned down because the server was not
// in a state to accept it yet, e.g. because it is running through its power on
// self test. Unlike a rejection, it is answered by establishing the state the
// request needs rather than by giving up on it.
type ErrNotSettled struct {
	innerErr error
}

// NewNotSettledErr wraps the provided error as an ErrNotSettled, if the passed
// err is none nil. If the passed err is nil, this function does not wrap and
// returns nil.
func NewNotSettledErr(err error) error {
	if err == nil {
		return nil
	}

	return ErrNotSettled{
		innerErr: err,
	}
}

func (e ErrNotSettled) Error() string {
	return fmt.Sprintf("Not settled: %v", e.innerErr.Error())
}

func (e ErrNotSettled) Unwrap() error {
	return e.innerErr
}

func IsNotSettledError(err error) bool {
	var notSettledErr ErrNotSettled
	return errors.As(err, &notSettledErr)
}

// Incus client returns connection errors with "Unable to connect to" prefix
// see: https://github.com/lxc/incus/blob/07852cf61699581d05649eab55b02bc7aff7e68f/shared/tls/tls.go#L19
// The original error can not be matched other than string comparison.
var retryableIncusConnectErrors = regexp.MustCompile(`context deadline exceeded|Unable to connect to:.*\(.*(context cancelled|connection refused).*\)`)

func RetryableWrapper() func(err error) error {
	return func(err error) error {
		if err == nil {
			return nil
		}

		// Connection errors are retryable.
		if errors.Is(err, syscall.ECONNREFUSED) ||
			errors.Is(err, io.EOF) ||
			errors.Is(err, io.ErrUnexpectedEOF) {
			return NewRetryableErr(err)
		}

		// Cancelled context or context with exceeded deadline are retryable.
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled) {
			return NewRetryableErr(err)
		}

		// Retryable incus errors.
		if strings.Contains(err.Error(), "no available cowsql leader server found") {
			return NewRetryableErr(err)
		}

		// An Incus daemon, that is still starting up, rejects the request. It is
		// reported either verbatim or as the transport symptom of the rejected
		// upgrade of the connection, e.g. while an instance is migrated to a
		// cluster member, whose daemon has not finished starting yet.
		if strings.Contains(err.Error(), "Daemon is starting up") ||
			strings.Contains(err.Error(), "websocket: bad handshake") {
			return NewRetryableErr(err)
		}

		if retryableIncusConnectErrors.MatchString(err.Error()) {
			return NewRetryableErr(err)
		}

		return err
	}
}
