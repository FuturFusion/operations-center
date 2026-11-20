package oidc

import (
	"context"
	"testing"
	"time"

	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/op"
)

func TestVerifierRetryGetAccessTokenVerifier(t *testing.T) {
	t.Parallel()

	m, err := mockoidc.Run()
	require.NoError(t, err)

	// Close instead of Shutdown. Shutdown waits 5 seconds for new idle connections.
	t.Cleanup(func() {
		err := m.Server.Close()
		require.NoError(t, err)
	})

	initialized := &op.AccessTokenVerifier{}

	tests := []struct {
		name       string
		issuer     string
		initial    *op.AccessTokenVerifier
		newContext func(t *testing.T) context.Context

		assertVerifier func(tt require.TestingT, got *op.AccessTokenVerifier)
	}{
		{
			name:    "issuer reachable on retry",
			issuer:  m.Issuer(),
			initial: nil,
			newContext: func(t *testing.T) context.Context {
				t.Helper()

				return t.Context()
			},

			assertVerifier: func(tt require.TestingT, got *op.AccessTokenVerifier) {
				require.NotNil(tt, got, "Retry must set the verifier")
				require.Equal(tt, m.Issuer(), got.Issuer)
			},
		},
		{
			name:    "verifier initialized in the meantime",
			issuer:  m.Issuer(),
			initial: initialized,
			newContext: func(t *testing.T) context.Context {
				t.Helper()

				return t.Context()
			},

			assertVerifier: func(tt require.TestingT, got *op.AccessTokenVerifier) {
				require.Same(tt, initialized, got, "Retry must keep the verifier set in the meantime")
			},
		},
		{
			name:    "issuer not reachable until context ends",
			issuer:  "http://127.0.0.1:1",
			initial: nil,
			newContext: func(t *testing.T) context.Context {
				t.Helper()

				ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
				t.Cleanup(cancel)

				return ctx
			},

			assertVerifier: func(tt require.TestingT, got *op.AccessTokenVerifier) {
				require.Nil(tt, got, "Failed retries must not set the verifier")
			},
		},
		{
			name:    "context cancelled",
			issuer:  m.Issuer(),
			initial: nil,
			newContext: func(t *testing.T) context.Context {
				t.Helper()

				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},

			assertVerifier: func(tt require.TestingT, got *op.AccessTokenVerifier) {
				require.Nil(tt, got, "A cancelled retry must not set the verifier")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier := &Verifier{
				issuer:        tc.issuer,
				state:         &verifierState{accessTokenVerifier: tc.initial},
				retryInterval: time.Millisecond,
			}

			// Run test
			verifier.retryGetAccessTokenVerifier(tc.newContext(t))

			// Assert
			tc.assertVerifier(t, verifier.state.accessTokenVerifier)
		})
	}
}
