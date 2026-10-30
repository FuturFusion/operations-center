package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authz"
)

type staticAuthorizer struct {
	err error
}

func (s staticAuthorizer) CheckPermission(context.Context, *authz.RequestDetails, authz.Object, authz.Entitlement) error {
	return s.err
}

func TestReplaceable_concurrent(t *testing.T) {
	denied := errors.New("denied")
	authorizer := authz.NewReplaceable(staticAuthorizer{err: denied})

	done := make(chan struct{})
	go func() {
		defer close(done)

		for range 100 {
			authorizer.Replace(staticAuthorizer{})
		}
	}()

	for range 100 {
		err := authorizer.CheckPermission(t.Context(), nil, authz.Object(""), authz.EntitlementCanView)
		if err != nil {
			require.ErrorIs(t, err, denied)
		}
	}

	<-done

	require.NoError(t, authorizer.CheckPermission(t.Context(), nil, authz.Object(""), authz.EntitlementCanView))
}
