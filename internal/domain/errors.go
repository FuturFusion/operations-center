package domain

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	return fmt.Sprintf("Retryable: %v", e.innerErr.Error())
}

func (e ErrRetryable) Unwrap() error {
	return e.innerErr
}

func IsRetryableError(err error) bool {
	var retryableErr ErrRetryable
	return errors.As(err, &retryableErr)
}

func RetryableWrapper() func(err error) error {
	return func(err error) error {
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

		return err
	}
}
