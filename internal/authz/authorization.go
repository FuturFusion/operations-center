package authz

import (
	"context"
	"net/http"
)

// Authorizer is the primary external API for this package.
type Authorizer interface {
	CheckPermission(ctx context.Context, r *http.Request, object Object, entitlement Entitlement) error
}
