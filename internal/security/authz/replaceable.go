package authz

import (
	"context"
	"sync/atomic"
)

// Replaceable is an Authorizer whose implementation can be replaced while requests are checked.
type Replaceable struct {
	current atomic.Pointer[Authorizer]
}

// NewReplaceable returns a Replaceable using authorizer.
func NewReplaceable(authorizer Authorizer) *Replaceable {
	r := &Replaceable{}
	r.Replace(authorizer)

	return r
}

// Replace makes r check the next requests with authorizer.
func (r *Replaceable) Replace(authorizer Authorizer) {
	r.current.Store(&authorizer)
}

// CheckPermission implements [Authorizer].
func (r *Replaceable) CheckPermission(ctx context.Context, details *RequestDetails, object Object, entitlement Entitlement) error {
	return (*r.current.Load()).CheckPermission(ctx, details, object, entitlement)
}
