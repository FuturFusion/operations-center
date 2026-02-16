package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound = errors.New("Not found")

	ErrConstraintViolation = errors.New("Constraint violation")

	ErrOperationNotPermitted = errors.New("Operation not permitted")

	ErrNotAuthenticated = errors.New("Not authenticated")

	ErrNotAuthorized = errors.New("Not authorized")
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
