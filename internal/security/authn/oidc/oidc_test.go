package oidc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestOIDC_Auth(t *testing.T) {
	t.Parallel()

	iss := newFakeIssuer(t)

	validToken := iss.sign(t, nil)
	otherClient := iss.sign(t, jwt.MapClaims{"aud": []string{"other"}})

	newVerifier := func(audience string, claim string) func(t *testing.T) *Verifier {
		return func(t *testing.T) *Verifier {
			t.Helper()

			verifier, err := NewVerifier(t.Context(), iss.URL, testClientID, "", audience, claim)
			require.NoError(t, err, "create verifier")

			return verifier
		}
	}

	tests := []struct {
		name          string
		verifier      func(t *testing.T) *Verifier
		authorization string
		cookies       map[string]string

		assertErr    require.ErrorAssertionFunc
		wantTrusted  bool
		wantUser     string
		wantProtocol string
		wantHeader   http.Header
	}{
		{
			name:          "success - bearer token",
			verifier:      newVerifier("", ""),
			authorization: "Bearer " + validToken,

			assertErr:    require.NoError,
			wantTrusted:  true,
			wantUser:     testSubject,
			wantProtocol: api.AuthenticationMethodOIDC,
			wantHeader:   http.Header{},
		},
		{
			name:     "success - access cookie",
			verifier: newVerifier("", ""),
			cookies:  map[string]string{"oidc_access": validToken},

			assertErr:    require.NoError,
			wantTrusted:  true,
			wantUser:     testSubject,
			wantProtocol: api.AuthenticationMethodOIDC,
			wantHeader:   http.Header{},
		},
		{
			name: "no verifier",
			verifier: func(t *testing.T) *Verifier {
				t.Helper()

				return nil
			},
			authorization: "Bearer " + validToken,

			assertErr:    require.NoError,
			wantTrusted:  false,
			wantUser:     "",
			wantProtocol: "",
			wantHeader:   http.Header{},
		},
		{
			name:     "no OIDC request",
			verifier: newVerifier("", ""),

			assertErr:    require.NoError,
			wantTrusted:  false,
			wantUser:     "",
			wantProtocol: "",
			wantHeader:   http.Header{},
		},
		{
			name:          "error - invalid token writes OIDC headers",
			verifier:      newVerifier("https://api.example.com", ""),
			authorization: "Bearer " + otherClient,

			assertErr:    requireAuthError,
			wantTrusted:  false,
			wantUser:     "",
			wantProtocol: "",
			wantHeader: http.Header{
				"X-Operationscenter-Oidc-Issuer":   {iss.URL},
				"X-Operationscenter-Oidc-Clientid": {testClientID},
				"X-Operationscenter-Oidc-Audience": {"https://api.example.com"},
			},
		},
		{
			name:          "error - configured claim missing writes no OIDC headers",
			verifier:      newVerifier("", "preferred_username"),
			authorization: "Bearer " + validToken,

			assertErr:    requireNotAuthError,
			wantTrusted:  false,
			wantUser:     "",
			wantProtocol: "",
			wantHeader:   http.Header{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			auther := New(tc.verifier(t))

			req := httptest.NewRequest(http.MethodGet, "/1.0", http.NoBody)
			req.Header.Set("Authorization", tc.authorization)
			for name, value := range tc.cookies {
				req.AddCookie(&http.Cookie{Name: name, Value: value})
			}

			rec := httptest.NewRecorder()

			// Run test
			trusted, user, protocol, err := auther.Auth(rec, req)

			// Assert
			tc.assertErr(t, err, "error of Auth")
			require.Equal(t, tc.wantTrusted, trusted, "trusted")
			require.Equal(t, tc.wantUser, user, "user name")
			require.Equal(t, tc.wantProtocol, protocol, "authentication protocol")
			require.Equal(t, tc.wantHeader, rec.Header(), "response headers")
		})
	}
}
