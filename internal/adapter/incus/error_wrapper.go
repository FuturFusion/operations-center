package incus

import (
	"net/http"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// notFoundError is an Incus "not found" error, that matches domain.ErrNotFound.
// It keeps the text of the Incus error and the Incus error in the chain.
type notFoundError struct {
	err error
}

func (e notFoundError) Error() string {
	return e.err.Error()
}

func (e notFoundError) Unwrap() error {
	return e.err
}

func (e notFoundError) Is(target error) bool {
	return target == domain.ErrNotFound
}

// NotFoundWrapper maps Incus "not found" errors to domain.ErrNotFound.
// The message of the Incus error stays the message for the user.
// The original error stays in the chain. The API reports these errors as
// upstream errors, see response.SmartError.
//
// The errors of the Wait methods of returned operations are not mapped.
// Incus reports a failed operation as text without a status code.
func NotFoundWrapper() func(err error) error {
	return func(err error) error {
		if incusapi.StatusErrorCheck(err, http.StatusNotFound) {
			return notFoundError{err: err}
		}

		return err
	}
}
