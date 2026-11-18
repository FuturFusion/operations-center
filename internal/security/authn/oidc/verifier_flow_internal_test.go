package oidc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifierRetryGetAccessTokenVerifier_cancelled(t *testing.T) {
	t.Parallel()

	// Setup
	verifier := &Verifier{issuer: "http://127.0.0.1:1", state: &verifierState{}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Run test
	verifier.retryGetAccessTokenVerifier(ctx)

	// Assert
	require.Nil(t, verifier.state.accessTokenVerifier, "A cancelled retry must not set the verifier")
}
