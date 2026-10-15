package chain_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/security/authz/chain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type authorizerFunc func() error

func (f authorizerFunc) CheckPermission(_ context.Context, _ *authz.RequestDetails, _ authz.Object, _ authz.Entitlement) error {
	return f()
}

func TestChain_CheckPermission(t *testing.T) {
	t.Parallel()

	grant := func(calls *int) authz.Authorizer {
		return authorizerFunc(func() error {
			*calls++
			return nil
		})
	}

	reject := func(calls *int) authz.Authorizer {
		return authorizerFunc(func() error {
			*calls++
			return errors.New("rejected")
		})
	}

	deny := func(calls *int) authz.Authorizer {
		return authorizerFunc(func() error {
			*calls++
			return authz.Deny(errors.New("denied"))
		})
	}

	tests := []struct {
		name        string
		authorizers []func(calls *int) authz.Authorizer

		assertErr require.ErrorAssertionFunc
		wantCalls []int
	}{
		{
			name: "no authorizer",

			assertErr: forbidden,
			wantCalls: []int{},
		},
		{
			name:        "first authorizer grants",
			authorizers: []func(calls *int) authz.Authorizer{grant, grant},

			assertErr: require.NoError,
			wantCalls: []int{1, 0},
		},
		{
			name:        "second authorizer grants after a rejection",
			authorizers: []func(calls *int) authz.Authorizer{reject, grant},

			assertErr: require.NoError,
			wantCalls: []int{1, 1},
		},
		{
			name:        "all authorizers reject",
			authorizers: []func(calls *int) authz.Authorizer{reject, reject},

			assertErr: forbidden,
			wantCalls: []int{1, 1},
		},
		{
			name:        "final denial stops the chain",
			authorizers: []func(calls *int) authz.Authorizer{deny, grant},

			assertErr: forbidden,
			wantCalls: []int{1, 0},
		},
		{
			name:        "final denial after a rejection",
			authorizers: []func(calls *int) authz.Authorizer{reject, deny, grant},

			assertErr: forbidden,
			wantCalls: []int{1, 1, 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			calls := make([]int, len(tc.authorizers))
			authorizers := make([]authz.Authorizer, 0, len(tc.authorizers))
			for i, newAuthorizer := range tc.authorizers {
				authorizers = append(authorizers, newAuthorizer(&calls[i]))
			}

			c := chain.New(authorizers...)

			// Run test
			err := c.CheckPermission(t.Context(), &authz.RequestDetails{}, authz.ObjectServer(), authz.EntitlementCanView)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantCalls, calls, "expect the authorizers to be asked in order until one grants or denies finally")
		})
	}
}

func forbidden(t require.TestingT, err error, _ ...any) {
	require.True(t, api.StatusErrorCheck(err, http.StatusForbidden), "expect a forbidden error, got: %v", err)
}
