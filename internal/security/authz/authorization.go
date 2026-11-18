package authz

import (
	"context"
	"errors"
)

// Authorizer is the primary external API for this package.
type Authorizer interface {
	CheckPermission(ctx context.Context, details *RequestDetails, object Object, entitlement Entitlement) error
}

// Deny marks err as a final denial. The chain authorizer does not ask the
// following authorizers after a final denial.
func Deny(err error) error {
	return deniedError{err: err}
}

// IsDenied reports whether err is a final denial.
func IsDenied(err error) bool {
	var denied deniedError

	return errors.As(err, &denied)
}

type deniedError struct {
	err error
}

func (e deniedError) Error() string {
	return e.err.Error()
}

func (e deniedError) Unwrap() error {
	return e.err
}
