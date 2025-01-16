package operations

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("Not found")

var ErrConstraintViolation = errors.New("Constraint violation")

type ErrValidation string

func NewValidationErrf(format string, a ...any) error {
	return ErrValidation(fmt.Sprintf(format, a...))
}

func (e ErrValidation) Error() string {
	return string(e)
}
