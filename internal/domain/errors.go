package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound = errors.New("Not found")

	ErrConstraintViolation = errors.New("Constraint violation")

	ErrOperationNotPermitted = errors.New("Operation not permitted")
)

type ErrValidation string

func NewValidationErrf(format string, a ...any) error {
	return ErrValidation(fmt.Sprintf(format, a...))
}

func (e ErrValidation) Error() string {
	return string(e)
}
