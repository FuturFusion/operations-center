package chain

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Chain represents a chain of authorizers. They are probed in order.
// Processing is stopped after the first authorizer returns without error
// and access is granted.
// If all authorizers fail, an error is returned and access is forbidden.
type Chain struct {
	authorizers []authz.Authorizer
}

func New(authorizers ...authz.Authorizer) Chain {
	return Chain{
		authorizers: authorizers,
	}
}

func (c Chain) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	errs := make([]error, 0, len(c.authorizers))

	for _, authorizer := range c.authorizers {
		err := authorizer.CheckPermission(ctx, details, object, entitlement)
		if err == nil {
			return nil
		}

		errs = append(errs, err)
	}

	slog.DebugContext(ctx, "chain authorizer failed", logger.Err(errors.Join(errs...)), slog.String("user", details.Username), slog.String("entitlement", entitlement.String()), slog.String("object", object.String()))

	return api.StatusErrorf(http.StatusForbidden, "User does not have entitlement %q on object %q", entitlement, object)
}
