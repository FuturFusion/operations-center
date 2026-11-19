package incus

import (
	"net/http"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// NotFoundWrapper maps Incus "not found" errors to domain.ErrNotFound.
// The original error stays in the chain.
func NotFoundWrapper() func(err error) error {
	return func(err error) error {
		if incusapi.StatusErrorCheck(err, http.StatusNotFound) {
			return domain.NewErrorf(domain.ErrNotFound, "", "Resource not found on the Incus server").WithCause(err)
		}

		return err
	}
}
