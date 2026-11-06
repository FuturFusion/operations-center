package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

func TestCheckTokenBinding(t *testing.T) {
	const clientID = "operations-center"

	tests := []struct {
		name     string
		claims   oidc.TokenClaims
		audience string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:   "success - client ID in audience",
			claims: oidc.TokenClaims{Audience: oidc.Audience{"other", clientID}},

			assertErr: require.NoError,
		},
		{
			name:   "success - client ID in azp",
			claims: oidc.TokenClaims{Audience: oidc.Audience{"account"}, AuthorizedParty: clientID},

			assertErr: require.NoError,
		},
		{
			name:   "success - client ID in client_id",
			claims: oidc.TokenClaims{Audience: oidc.Audience{"api"}, ClientID: clientID},

			assertErr: require.NoError,
		},
		{
			name:     "success - configured audience and client ID in azp",
			claims:   oidc.TokenClaims{Audience: oidc.Audience{"https://api.example.com"}, AuthorizedParty: clientID},
			audience: "https://api.example.com",

			assertErr: require.NoError,
		},
		{
			name:   "error - token issued for other client",
			claims: oidc.TokenClaims{Audience: oidc.Audience{"other"}, AuthorizedParty: "other", ClientID: "other"},

			assertErr: require.Error,
		},
		{
			name:   "error - no audience and no client claims",
			claims: oidc.TokenClaims{},

			assertErr: require.Error,
		},
		{
			name:     "error - configured audience missing",
			claims:   oidc.TokenClaims{Audience: oidc.Audience{clientID}, AuthorizedParty: clientID},
			audience: "https://api.example.com",

			assertErr: require.Error,
		},
		{
			name:     "error - configured audience present but token issued for other client",
			claims:   oidc.TokenClaims{Audience: oidc.Audience{"https://api.example.com"}, AuthorizedParty: "other"},
			audience: "https://api.example.com",

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			claims := &oidc.AccessTokenClaims{TokenClaims: tc.claims}

			// Run test
			err := checkTokenBinding(claims, clientID, tc.audience)

			// Assert
			tc.assertErr(t, err, "unexpected result for token binding check")
		})
	}
}

func TestCheckTokenBinding_emptyClientID(t *testing.T) {
	t.Parallel()

	// Setup
	claims := &oidc.AccessTokenClaims{TokenClaims: oidc.TokenClaims{Audience: oidc.Audience{""}}}

	// Run test
	err := checkTokenBinding(claims, "", "")

	// Assert
	require.Error(t, err, "an empty client ID must never match")
}
