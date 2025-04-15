package chain

import (
	"context"
	"errors"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Chain represents an unix socket authorizer.
type Chain struct {
	authz.CommonAuthorizer

	authorizers []authz.Authorizer
}

func New(authorizers ...authz.Authorizer) Chain {
	return Chain{
		authorizers: authorizers,
	}
}

func (c Chain) CheckPermission(ctx context.Context, r *http.Request, object authz.Object, entitlement authz.Entitlement) error {
	errs := []error{
		api.StatusErrorf(http.StatusForbidden, "User does not have entitlement %q on object %q", entitlement, object),
	}

	for _, authorizer := range c.authorizers {
		err := authorizer.CheckPermission(ctx, r, object, entitlement)
		if err == nil {
			return nil
		}

		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
