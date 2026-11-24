package oidc

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
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

func requireAuthError(t require.TestingT, err error, msgAndArgs ...any) {
	var authErr *AuthError
	require.ErrorAs(t, err, &authErr, msgAndArgs...)
}

// Compare with the sentinel errors of the library. Its error texts can change.
func requireAuthErrorIs(target error) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, msgAndArgs ...any) {
		requireAuthError(t, err, msgAndArgs...)
		require.ErrorIs(t, err, target, msgAndArgs...)
	}
}

func requireNotAuthError(t require.TestingT, err error, msgAndArgs ...any) {
	var authErr *AuthError
	require.Error(t, err, msgAndArgs...)
	require.NotErrorAs(t, err, &authErr, msgAndArgs...)
}

func TestVerifier_Auth(t *testing.T) {
	t.Parallel()

	iss := newFakeIssuer(t)
	deadIssuer := unreachableIssuer(t)

	now := time.Now()
	withEmail := iss.sign(t, jwt.MapClaims{"email": "user@example.com"})
	withoutEmail := iss.sign(t, nil)
	withEmptyEmail := iss.sign(t, jwt.MapClaims{"email": ""})
	withoutSubject := iss.sign(t, jwt.MapClaims{"sub": ""})
	withClaim := iss.sign(t, jwt.MapClaims{"preferred_username": "jdoe"})
	withNumberClaim := iss.sign(t, jwt.MapClaims{"preferred_username": 42})
	expired := iss.sign(t, jwt.MapClaims{"iat": now.Add(-2 * time.Hour).Unix(), "exp": now.Add(-time.Hour).Unix()})
	otherClient := iss.sign(t, jwt.MapClaims{"aud": []string{"other"}})
	refreshed := iss.sign(t, jwt.MapClaims{"sub": "refreshed"})
	wrongIssuer := iss.sign(t, jwt.MapClaims{"iss": "https://other.example.com"})
	withoutExpiry := iss.sign(t, jwt.MapClaims{"exp": nil})
	foreignKey := newFakeIssuer(t).sign(t, jwt.MapClaims{"iss": iss.URL})
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": iss.URL,
		"sub": testSubject,
		"aud": []string{testClientID},
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err, "create unsigned token")

	iss.tokenResponses["refresh-ok"] = map[string]any{"access_token": refreshed, "token_type": "Bearer", "refresh_token": "refresh-new", "expires_in": 3600}
	iss.tokenResponses["refresh-keep"] = map[string]any{"access_token": refreshed, "token_type": "Bearer", "expires_in": 3600}
	iss.tokenResponses["refresh-other-client"] = map[string]any{"access_token": otherClient, "token_type": "Bearer", "expires_in": 3600}

	tests := []struct {
		name          string
		issuer        string
		claim         string
		authorization string
		cookies       map[string]string

		assertErr   require.ErrorAssertionFunc
		wantUser    string
		wantCookies map[string]string
	}{
		{
			name:          "success - bearer token with email",
			issuer:        iss.URL,
			authorization: "Bearer " + withEmail,

			assertErr:   require.NoError,
			wantUser:    "user@example.com",
			wantCookies: map[string]string{},
		},
		{
			name:    "success - access cookie without email",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": withoutEmail},

			assertErr:   require.NoError,
			wantUser:    testSubject,
			wantCookies: map[string]string{},
		},
		{
			name:          "success - empty email falls back to subject",
			issuer:        iss.URL,
			authorization: "Bearer " + withEmptyEmail,

			assertErr:   require.NoError,
			wantUser:    testSubject,
			wantCookies: map[string]string{},
		},
		{
			name:          "success - configured claim",
			issuer:        iss.URL,
			claim:         "preferred_username",
			authorization: "Bearer " + withClaim,

			assertErr:   require.NoError,
			wantUser:    "jdoe",
			wantCookies: map[string]string{},
		},
		{
			name:    "success - refresh with new refresh token",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": expired, "oidc_refresh": "refresh-ok"},

			assertErr:   require.NoError,
			wantUser:    "refreshed",
			wantCookies: map[string]string{"oidc_access": refreshed, "oidc_refresh": "refresh-new"},
		},
		{
			name:    "success - refresh without new refresh token",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": expired, "oidc_refresh": "refresh-keep"},

			assertErr:   require.NoError,
			wantUser:    "refreshed",
			wantCookies: map[string]string{"oidc_access": refreshed},
		},
		{
			name:          "error - not a bearer token",
			issuer:        iss.URL,
			authorization: "Basic dXNlcjpwYXNz",

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:   "error - no token",
			issuer: iss.URL,

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - signed with foreign key",
			issuer:        iss.URL,
			authorization: "Bearer " + foreignKey,

			assertErr:   requireAuthErrorIs(oidc.ErrSignatureInvalid),
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - unsigned token with alg none",
			issuer:        iss.URL,
			authorization: "Bearer " + unsigned,

			assertErr:   requireAuthErrorIs(oidc.ErrSignatureUnsupportedAlg),
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - wrong issuer",
			issuer:        iss.URL,
			authorization: "Bearer " + wrongIssuer,

			assertErr:   requireAuthErrorIs(oidc.ErrIssuerInvalid),
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - no expiry",
			issuer:        iss.URL,
			authorization: "Bearer " + withoutExpiry,

			assertErr:   requireAuthErrorIs(oidc.ErrExpired),
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:    "error - expired without refresh token",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": expired},

			assertErr:   requireAuthErrorIs(oidc.ErrExpired),
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - malformed token",
			issuer:        iss.URL,
			authorization: "Bearer not-a-token",

			assertErr:   requireAuthErrorIs(oidc.ErrParse),
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - configured claim missing",
			issuer:        iss.URL,
			claim:         "preferred_username",
			authorization: "Bearer " + withEmail,

			assertErr:   requireNotAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - configured claim not a string",
			issuer:        iss.URL,
			claim:         "preferred_username",
			authorization: "Bearer " + withNumberClaim,

			assertErr:   requireNotAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - no subject and no email",
			issuer:        iss.URL,
			authorization: "Bearer " + withoutSubject,

			assertErr:   requireNotAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:          "error - token for other client without refresh token",
			issuer:        iss.URL,
			authorization: "Bearer " + otherClient,

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:    "error - refresh rejected by issuer",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": expired, "oidc_refresh": "refresh-unknown"},

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:    "error - refreshed token for other client",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": expired, "oidc_refresh": "refresh-other-client"},

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
		{
			name:    "error - issuer unreachable",
			issuer:  deadIssuer,
			cookies: map[string]string{"oidc_access": expired, "oidc_refresh": "refresh-ok"},

			assertErr:   requireAuthError,
			wantUser:    "",
			wantCookies: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := NewVerifier(t.Context(), tc.issuer, testClientID, "", "", tc.claim)
			require.NoError(t, err, "create verifier")

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.Header.Set("Authorization", tc.authorization)
			for name, value := range tc.cookies {
				req.AddCookie(&http.Cookie{Name: name, Value: value})
			}

			rec := httptest.NewRecorder()

			// Run test
			user, err := verifier.Auth(t.Context(), rec, req)

			// Assert
			tc.assertErr(t, err, "error of Auth")
			require.Equal(t, tc.wantUser, user, "user name")
			require.Equal(t, tc.wantCookies, responseCookies(rec), "cookies set on the response")
		})
	}
}

func TestVerifier_Auth_refreshWithoutResponseWriter(t *testing.T) {
	t.Parallel()

	// Setup
	iss := newFakeIssuer(t)
	expired := iss.sign(t, jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()})
	iss.tokenResponses["refresh-ok"] = map[string]any{"access_token": iss.sign(t, nil), "token_type": "Bearer", "refresh_token": "refresh-new"}

	verifier, err := NewVerifier(t.Context(), iss.URL, testClientID, "", "", "")
	require.NoError(t, err, "create verifier")

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "oidc_access", Value: expired})
	req.AddCookie(&http.Cookie{Name: "oidc_refresh", Value: "refresh-ok"})

	// Run test
	user, err := verifier.Auth(t.Context(), nil, req)

	// Assert
	require.NoError(t, err, "refresh must work without a response writer")
	require.Equal(t, testSubject, user, "user name from the refreshed token")
}

func TestVerifier_Auth_configuredAudience(t *testing.T) {
	t.Parallel()

	const audience = "https://api.example.com"

	iss := newFakeIssuer(t)
	withAudience := iss.sign(t, jwt.MapClaims{"aud": []string{audience}, "azp": testClientID})
	withoutAudience := iss.sign(t, nil)

	tests := []struct {
		name  string
		token string

		assertErr require.ErrorAssertionFunc
		wantUser  string
	}{
		{
			name:  "success - token for the configured audience",
			token: withAudience,

			assertErr: require.NoError,
			wantUser:  testSubject,
		},
		{
			name:  "error - token without the configured audience",
			token: withoutAudience,

			assertErr: requireAuthError,
			wantUser:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := NewVerifier(t.Context(), iss.URL, testClientID, "", audience, "")
			require.NoError(t, err, "create verifier")

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.Header.Set("Authorization", "Bearer "+tc.token)

			// Run test
			user, err := verifier.Auth(t.Context(), nil, req)

			// Assert
			tc.assertErr(t, err, "error of Auth")
			require.Equal(t, tc.wantUser, user, "user name")
		})
	}
}

// The daemon uses a zero value Verifier until OIDC is configured.
func TestVerifier_Auth_zeroValue(t *testing.T) {
	t.Parallel()

	// Setup
	iss := newFakeIssuer(t)
	verifier := &Verifier{}

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+iss.sign(t, nil))

	// Run test
	user, err := verifier.Auth(t.Context(), nil, req)

	// Assert
	requireAuthErrorIs(oidc.ErrDiscoveryFailed)(t, err, "error of Auth")
	require.Empty(t, user, "user name")
}

func TestVerifier_Logout(t *testing.T) {
	t.Parallel()

	iss := newFakeIssuer(t)
	deadIssuer := unreachableIssuer(t)

	tests := []struct {
		name    string
		issuer  string
		cookies map[string]string
		idToken string

		wantEndSession bool
	}{
		{
			name:    "no ID token",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_access": "access"},
			idToken: "",

			wantEndSession: false,
		},
		{
			name:    "ID token ends the session at the issuer",
			issuer:  iss.URL,
			cookies: map[string]string{"oidc_id": "id-token-1"},
			idToken: "id-token-1",

			wantEndSession: true,
		},
		{
			name:    "issuer unreachable",
			issuer:  deadIssuer,
			cookies: map[string]string{"oidc_id": "id-token-2"},
			idToken: "id-token-2",

			wantEndSession: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := NewVerifier(t.Context(), tc.issuer, testClientID, "", "", "")
			require.NoError(t, err, "create verifier")

			req := httptest.NewRequest(http.MethodPost, "/oidc/logout", http.NoBody)
			for name, value := range tc.cookies {
				req.AddCookie(&http.Cookie{Name: name, Value: value})
			}

			rec := httptest.NewRecorder()

			// Run test
			verifier.Logout(rec, req)

			// Assert
			require.Equal(t, map[string]string{"oidc_access": "", "oidc_id": "", "oidc_refresh": ""}, responseCookies(rec), "all OIDC cookies are cleared")
			for _, cookie := range rec.Result().Cookies() {
				require.True(t, cookie.Expires.Equal(time.Unix(0, 0)), "cookie %q must expire", cookie.Name)
			}

			require.Equal(t, tc.wantEndSession, iss.endSessionCalled(tc.idToken), "session ended at the issuer")
		})
	}
}

func TestVerifier_Login(t *testing.T) {
	t.Parallel()

	iss := newFakeIssuer(t)
	deadIssuer := unreachableIssuer(t)

	tests := []struct {
		name   string
		issuer string

		wantStatus      int
		wantLocation    string
		wantQuery       url.Values
		randomQueryKeys []string
		wantCookieNames []string
	}{
		{
			name:   "success - redirect to the issuer",
			issuer: iss.URL,

			wantStatus:   http.StatusFound,
			wantLocation: iss.URL + "/authorize",
			wantQuery: url.Values{
				"audience":              {"https://api.example.com"},
				"client_id":             {testClientID},
				"code_challenge_method": {"S256"},
				"redirect_uri":          {"https://example.com/oidc/callback"},
				"response_type":         {"code"},
				"scope":                 {"openid offline_access"},
			},
			randomQueryKeys: []string{"code_challenge", "state"},
			wantCookieNames: []string{"pkce", "state"},
		},
		{
			name:   "error - issuer unreachable",
			issuer: deadIssuer,

			wantStatus:      http.StatusBadGateway,
			wantLocation:    "",
			wantQuery:       url.Values{},
			randomQueryKeys: []string{},
			wantCookieNames: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := NewVerifier(t.Context(), tc.issuer, testClientID, "", "https://api.example.com", "")
			require.NoError(t, err, "create verifier")

			req := httptest.NewRequest(http.MethodGet, "/oidc/login", http.NoBody)
			rec := httptest.NewRecorder()

			// Run test
			verifier.Login(rec, req)

			// Assert
			require.Equal(t, tc.wantStatus, rec.Code, "status code, body: %s", rec.Body.String())

			location, err := url.Parse(rec.Header().Get("Location"))
			require.NoError(t, err, "parse redirect location")

			query := location.Query()
			for _, key := range tc.randomQueryKeys {
				require.NotEmpty(t, query.Get(key), "random query parameter %q", key)
				query.Del(key)
			}

			location.RawQuery = ""
			require.Equal(t, tc.wantLocation, location.String(), "redirect location without query")
			require.Equal(t, tc.wantQuery, query, "query parameters of the redirect")
			require.ElementsMatch(t, tc.wantCookieNames, slices.Collect(maps.Keys(responseCookies(rec))), "names of the cookies set on the response")
		})
	}
}

func TestVerifier_Callback(t *testing.T) {
	t.Parallel()

	iss := newFakeIssuer(t)
	deadIssuer := unreachableIssuer(t)

	accessToken := iss.sign(t, nil)
	idToken := iss.sign(t, nil)
	iss.tokenResponses["code-full"] = map[string]any{"access_token": accessToken, "token_type": "Bearer", "refresh_token": "refresh", "id_token": idToken, "expires_in": 3600}
	iss.tokenResponses["code-no-refresh"] = map[string]any{"access_token": accessToken, "token_type": "Bearer", "id_token": idToken, "expires_in": 3600}

	tests := []struct {
		name         string
		issuer       string
		query        string
		stateCookies map[string]string

		wantStatus   int
		wantLocation string
		wantCookies  map[string]string
	}{
		{
			name:         "success - all tokens",
			issuer:       iss.URL,
			query:        "code=code-full&state=state-1",
			stateCookies: map[string]string{"state": "state-1", "pkce": "verifier"},

			wantStatus:   http.StatusMovedPermanently,
			wantLocation: "/ui/",
			wantCookies:  map[string]string{"state": "", "pkce": "", "oidc_access": accessToken, "oidc_refresh": "refresh", "oidc_id": idToken},
		},
		{
			name:         "success - no refresh token",
			issuer:       iss.URL,
			query:        "code=code-no-refresh&state=state-1",
			stateCookies: map[string]string{"state": "state-1", "pkce": "verifier"},

			wantStatus:   http.StatusMovedPermanently,
			wantLocation: "/ui/",
			wantCookies:  map[string]string{"state": "", "pkce": "", "oidc_access": accessToken, "oidc_id": idToken},
		},
		{
			name:         "error - code rejected by issuer",
			issuer:       iss.URL,
			query:        "code=code-unknown&state=state-1",
			stateCookies: map[string]string{"state": "state-1", "pkce": "verifier"},

			wantStatus:   http.StatusUnauthorized,
			wantLocation: "",
			wantCookies:  map[string]string{"state": "", "pkce": ""},
		},
		{
			name:         "error - state cookie missing",
			issuer:       iss.URL,
			query:        "code=code-full&state=state-1",
			stateCookies: map[string]string{},

			wantStatus:   http.StatusUnauthorized,
			wantLocation: "",
			wantCookies:  map[string]string{},
		},
		{
			name:         "error - issuer unreachable",
			issuer:       deadIssuer,
			query:        "code=code-full&state=state-1",
			stateCookies: map[string]string{"state": "state-1", "pkce": "verifier"},

			wantStatus:   http.StatusBadGateway,
			wantLocation: "",
			wantCookies:  map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			verifier, err := NewVerifier(t.Context(), tc.issuer, testClientID, "", "", "")
			require.NoError(t, err, "create verifier")

			// The relying party stores state and PKCE (proof key for code exchange) in signed cookies.
			cookieHandler := httphelper.NewCookieHandler(verifier.cookieKey, verifier.cookieKey, httphelper.WithUnsecure())
			cookieRec := httptest.NewRecorder()
			for name, value := range tc.stateCookies {
				require.NoError(t, cookieHandler.SetCookie(cookieRec, name, value))
			}

			req := httptest.NewRequest(http.MethodGet, "/oidc/callback?"+tc.query, http.NoBody)
			for _, cookie := range cookieRec.Result().Cookies() {
				req.AddCookie(cookie)
			}

			rec := httptest.NewRecorder()

			// Run test
			verifier.Callback(rec, req)

			// Assert
			require.Equal(t, tc.wantStatus, rec.Code, "status code, body: %s", rec.Body.String())
			require.Equal(t, tc.wantLocation, rec.Header().Get("Location"), "redirect location")
			require.Equal(t, tc.wantCookies, responseCookies(rec), "cookies set on the response")
		})
	}
}

func TestVerifier_retryGetAccessTokenVerifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		initial           *op.AccessTokenVerifier
		discoveryFailures int32
		runFor            time.Duration

		wantInitialized    bool
		wantDiscoveryCalls int32
	}{
		{
			name:              "context canceled before first retry",
			initial:           nil,
			discoveryFailures: 0,
			runFor:            5 * time.Second,

			wantInitialized:    false,
			wantDiscoveryCalls: 0,
		},
		{
			name:              "already initialized by a request",
			initial:           &op.AccessTokenVerifier{},
			discoveryFailures: 0,
			runFor:            15 * time.Second,

			wantInitialized:    true,
			wantDiscoveryCalls: 0,
		},
		{
			name:              "issuer available on second retry",
			initial:           nil,
			discoveryFailures: 1,
			runFor:            25 * time.Second,

			wantInitialized:    true,
			wantDiscoveryCalls: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			iss := newFakeIssuer(t)
			iss.discoveryFailures.Store(tc.discoveryFailures)

			synctest.Test(t, func(t *testing.T) {
				// Setup
				verifier := &Verifier{issuer: iss.URL, state: &verifierState{accessTokenVerifier: tc.initial}}
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})

				// Run test
				go func() {
					verifier.retryGetAccessTokenVerifier(ctx)
					close(done)
				}()

				time.Sleep(tc.runFor)
				cancel()
				<-done

				// Assert
				require.Equal(t, tc.wantInitialized, verifier.state.accessTokenVerifier != nil, "access token verifier initialized")
				require.Equal(t, tc.wantDiscoveryCalls, iss.discoveryCalls.Load(), "calls of the discovery endpoint")
			})
		})
	}
}
