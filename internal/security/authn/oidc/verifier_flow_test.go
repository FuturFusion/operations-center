package oidc_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authn/oidc"
)

func TestVerifierAuth(t *testing.T) {
	t.Parallel()

	m := startMockOIDC(t)
	refreshToken := newRefreshToken(t, m, "refreshed-user")

	tests := []struct {
		name    string
		claim   string
		header  string
		cookies []*http.Cookie

		assertErr   require.ErrorAssertionFunc
		wantUser    string
		wantCookies []string
	}{
		{
			name:   "success - bearer token without email returns subject",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user"}),

			assertErr:   require.NoError,
			wantUser:    "subject-user",
			wantCookies: []string{},
		},
		{
			name:   "success - bearer token with email returns email",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user", "email": "user@example.com"}),

			assertErr:   require.NoError,
			wantUser:    "user@example.com",
			wantCookies: []string{},
		},
		{
			name:   "success - empty email falls back to subject",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user", "email": ""}),

			assertErr:   require.NoError,
			wantUser:    "subject-user",
			wantCookies: []string{},
		},
		{
			name:   "success - configured claim returns its value",
			claim:  "preferred_username",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user", "email": "user@example.com", "preferred_username": "claim-user"}),

			assertErr:   require.NoError,
			wantUser:    "claim-user",
			wantCookies: []string{},
		},
		{
			name:    "success - access token from cookie",
			cookies: []*http.Cookie{{Name: "oidc_access", Value: signToken(t, m, map[string]any{"sub": "cookie-user"})}},

			assertErr:   require.NoError,
			wantUser:    "cookie-user",
			wantCookies: []string{},
		},
		{
			name: "success - invalid access token is refreshed",
			cookies: []*http.Cookie{
				{Name: "oidc_access", Value: "invalid"},
				{Name: "oidc_refresh", Value: refreshToken},
			},

			assertErr:   require.NoError,
			wantUser:    "refreshed-user",
			wantCookies: []string{"oidc_access", "oidc_refresh"},
		},
		{
			name:   "error - authorization header without bearer token",
			header: "Basic dXNlcjpwYXNz",

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name: "error - no authorization header and no cookie",

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name:   "error - invalid access token without refresh cookie",
			header: "Bearer invalid",

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name:   "error - token issued for other client",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user", "aud": []string{"other-client"}}),

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name: "error - refresh token rejected by issuer",
			cookies: []*http.Cookie{
				{Name: "oidc_access", Value: "invalid"},
				{Name: "oidc_refresh", Value: "invalid"},
			},

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name:   "error - configured claim missing",
			claim:  "preferred_username",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user"}),

			assertErr:   requireNonAuthErrorContains(`missing required claim "preferred_username"`),
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name:   "error - configured claim not a string",
			claim:  "preferred_username",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user", "preferred_username": 42}),

			assertErr:   requireNonAuthErrorContains(`missing required claim "preferred_username"`),
			wantUser:    "",
			wantCookies: []string{},
		},
		{
			name:   "error - configured claim empty",
			claim:  "preferred_username",
			header: "Bearer " + signToken(t, m, map[string]any{"sub": "subject-user", "preferred_username": ""}),

			assertErr:   requireNonAuthErrorContains(`missing required claim "preferred_username"`),
			wantUser:    "",
			wantCookies: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := oidc.NewVerifier(t.Context(), m.Issuer(), m.ClientID, "", "", tc.claim)
			require.NoError(t, err)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", tc.header)
			for _, cookie := range tc.cookies {
				r.AddCookie(cookie)
			}

			w := httptest.NewRecorder()

			// Run test
			user, err := verifier.Auth(t.Context(), w, r)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantUser, user, "Unexpected user name")
			require.Equal(t, tc.wantCookies, cookieNames(w.Result().Cookies()), "Unexpected cookies on the response")
		})
	}
}

func TestVerifierLogout(t *testing.T) {
	t.Parallel()

	m := startMockOIDC(t)

	tests := []struct {
		name    string
		issuer  string
		cookies []*http.Cookie
	}{
		{
			name:   "issuer reachable with ID token",
			issuer: m.Issuer(),
			cookies: []*http.Cookie{
				{Name: "oidc_id", Value: signToken(t, m, map[string]any{"sub": "subject-user"})},
			},
		},
		{
			name:    "issuer reachable without ID token",
			issuer:  m.Issuer(),
			cookies: []*http.Cookie{},
		},
		{
			name:    "issuer not reachable",
			issuer:  "http://127.0.0.1:1",
			cookies: []*http.Cookie{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := oidc.NewVerifier(t.Context(), tc.issuer, m.ClientID, "", "", "")
			require.NoError(t, err)

			r := httptest.NewRequest(http.MethodGet, "/oidc/logout", nil)
			for _, cookie := range tc.cookies {
				r.AddCookie(cookie)
			}

			w := httptest.NewRecorder()

			// Run test
			verifier.Logout(w, r)

			// Assert
			cookies := w.Result().Cookies()
			require.Equal(t, []string{"oidc_access", "oidc_id", "oidc_refresh"}, cookieNames(cookies), "Logout must clear all OIDC cookies")
			for _, cookie := range cookies {
				require.Empty(t, cookie.Value, "Cookie %q must be cleared", cookie.Name)
				require.True(t, cookie.Expires.Before(time.Now()), "Cookie %q must be expired", cookie.Name)
			}
		})
	}
}

func TestVerifierLoginCallback(t *testing.T) {
	t.Parallel()

	// Setup
	m := startMockOIDC(t)
	// mockoidc does not support the default scope offline_access.
	verifier, err := oidc.NewVerifier(t.Context(), m.Issuer(), m.ClientID, "openid,email", "", "")
	require.NoError(t, err)

	loginResponse := httptest.NewRecorder()
	verifier.Login(loginResponse, httptest.NewRequest(http.MethodGet, "https://oc.example.com/oidc/login", nil))
	require.Equal(t, http.StatusFound, loginResponse.Code, "Login must redirect to the issuer")

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	authorizeResponse, err := noRedirect.Get(loginResponse.Header().Get("Location"))
	require.NoError(t, err)
	defer authorizeResponse.Body.Close()

	callbackURL, err := url.Parse(authorizeResponse.Header.Get("Location"))
	require.NoError(t, err)

	callbackRequest := httptest.NewRequest(http.MethodGet, "https://oc.example.com/oidc/callback?"+callbackURL.RawQuery, nil)
	for _, cookie := range loginResponse.Result().Cookies() {
		callbackRequest.AddCookie(cookie)
	}

	callbackResponse := httptest.NewRecorder()

	// Run test
	verifier.Callback(callbackResponse, callbackRequest)

	// Assert
	require.Equal(t, http.StatusMovedPermanently, callbackResponse.Code, "Callback must redirect to the UI: %s", callbackResponse.Body.String())
	require.Equal(t, "/ui/", callbackResponse.Header().Get("Location"))
	require.Equal(t, []string{"state", "pkce", "oidc_access", "oidc_refresh", "oidc_id"}, cookieNames(callbackResponse.Result().Cookies()), "Callback must clear the login flow cookies and store the tokens")
}

func TestVerifierCallback_issuerNotReachable(t *testing.T) {
	t.Parallel()

	// Setup
	verifier, err := oidc.NewVerifier(t.Context(), "http://127.0.0.1:1", "client", "", "", "")
	require.NoError(t, err)

	w := httptest.NewRecorder()

	// Run test
	verifier.Callback(w, httptest.NewRequest(http.MethodGet, "/oidc/callback", nil))

	// Assert
	require.Equal(t, http.StatusOK, w.Code, "Callback must not write a response")
	require.Empty(t, w.Result().Cookies(), "Callback must not set cookies")
}

// startMockOIDC starts a mock OIDC issuer.
// The verifier is a public client without a secret, but mockoidc requires one.
// A middleware adds the secret to every request.
func startMockOIDC(t *testing.T) *mockoidc.MockOIDC {
	t.Helper()

	m, err := mockoidc.NewServer(nil)
	require.NoError(t, err)

	err = m.AddMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			r.Form.Set("client_secret", m.ClientSecret)
			next.ServeHTTP(w, r)
		})
	})
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	err = m.Start(ln, nil)
	require.NoError(t, err)

	// Close instead of Shutdown. Shutdown waits 5 seconds for new idle connections.
	t.Cleanup(func() {
		err := m.Server.Close()
		require.NoError(t, err)
	})

	return m
}

// newRefreshToken returns a refresh token that the mock issuer accepts.
func newRefreshToken(t *testing.T, m *mockoidc.MockOIDC, subject string) string {
	t.Helper()

	m.QueueCode("refresh-" + subject)
	session, err := m.SessionStore.NewSession("openid", "", &mockoidc.MockUser{Subject: subject}, "", "")
	require.NoError(t, err)

	token, err := session.RefreshToken(m.Config(), m.Keypair, m.Now())
	require.NoError(t, err)

	return token
}

// signToken returns an access token signed by the mock issuer.
// The claims overwrite the defaults.
func signToken(t *testing.T, m *mockoidc.MockOIDC, claims map[string]any) string {
	t.Helper()

	now := time.Now()
	payload := map[string]any{
		"iss": m.Issuer(),
		"aud": []string{m.ClientID},
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(time.Minute).Unix(),
	}

	maps.Copy(payload, claims)

	kid, err := m.Keypair.KeyID()
	require.NoError(t, err)

	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	require.NoError(t, err)

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, m.Keypair.PrivateKey, crypto.SHA256, digest[:])
	require.NoError(t, err)

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func cookieNames(cookies []*http.Cookie) []string {
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		names = append(names, cookie.Name)
	}

	return names
}

// requireAuthError asserts an AuthError. OIDC.Auth only writes the OIDC headers for this error type.
func requireAuthError(tt require.TestingT, err error, a ...any) {
	var authErr *oidc.AuthError
	require.ErrorAs(tt, err, &authErr, a...)
}

func requireNonAuthErrorContains(contains string) require.ErrorAssertionFunc {
	return func(tt require.TestingT, err error, a ...any) {
		require.ErrorContains(tt, err, contains, a...)

		var authErr *oidc.AuthError
		require.NotErrorAs(tt, err, &authErr, a...)
	}
}
