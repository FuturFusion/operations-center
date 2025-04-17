package authz

import (
	"context"
)

// Authorizer is the primary external API for this package.
type Authorizer interface {
	CheckPermission(ctx context.Context, details *RequestDetails, object Object, entitlement Entitlement) error
}
