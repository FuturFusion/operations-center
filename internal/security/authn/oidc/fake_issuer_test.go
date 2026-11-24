package oidc

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"

	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
)

const (
	testClientID = "operations-center"
	testSubject  = "subject"
)

// fakeIssuer is a minimal OIDC issuer. mockoidc is not used as issuer,
// because its token endpoint requires a client secret.
type fakeIssuer struct {
	URL string

	keypair *mockoidc.Keypair

	// tokenResponses maps an authorization code or a refresh token to the
	// response of the token endpoint. Unknown keys get a 400 response.
	tokenResponses map[string]map[string]any

	// discoveryFailures is the number of discovery requests to fail.
	discoveryFailures atomic.Int32
	discoveryCalls    atomic.Int32

	endSessionHints sync.Map
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()

	keypair, err := mockoidc.RandomKeypair(2048)
	require.NoError(t, err, "create key pair")

	iss := &fakeIssuer{
		keypair:        keypair,
		tokenResponses: map[string]map[string]any{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", iss.discovery)
	mux.HandleFunc("GET /jwks", iss.jwks)
	mux.HandleFunc("POST /token", iss.token)
	mux.HandleFunc("POST /end-session", iss.endSession)

	// Without keep alive, no client connection stays open. An open
	// connection blocks the fake clock of synctest.
	server := httptest.NewUnstartedServer(mux)
	server.Config.SetKeepAlivesEnabled(false)
	server.Start()
	t.Cleanup(server.Close)

	iss.URL = server.URL

	return iss
}

func (iss *fakeIssuer) discovery(w http.ResponseWriter, _ *http.Request) {
	iss.discoveryCalls.Add(1)
	if iss.discoveryFailures.Add(-1) >= 0 {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                iss.URL,
		"authorization_endpoint":                iss.URL + "/authorize",
		"token_endpoint":                        iss.URL + "/token",
		"jwks_uri":                              iss.URL + "/jwks",
		"end_session_endpoint":                  iss.URL + "/end-session",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (iss *fakeIssuer) jwks(w http.ResponseWriter, _ *http.Request) {
	body, err := iss.keypair.JWKS()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (iss *fakeIssuer) token(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response, ok := iss.tokenResponses[r.Form.Get("code")+r.Form.Get("refresh_token")]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (iss *fakeIssuer) endSession(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	iss.endSessionHints.Store(r.Form.Get("id_token_hint"), struct{}{})
	http.Redirect(w, r, r.Form.Get("post_logout_redirect_uri"), http.StatusFound)
}

func (iss *fakeIssuer) endSessionCalled(idToken string) bool {
	_, ok := iss.endSessionHints.Load(idToken)
	return ok
}

// sign returns a token signed by the issuer. The extra claims override the
// defaults.
func (iss *fakeIssuer) sign(t *testing.T, extra jwt.MapClaims) string {
	t.Helper()

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": iss.URL,
		"sub": testSubject,
		"aud": []string{testClientID},
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}

	maps.Copy(claims, extra)

	token, err := iss.keypair.SignJWT(claims)
	require.NoError(t, err, "sign token")

	return token
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// unreachableIssuer returns a URL with a port that no server listens on.
// FreePort never returns a port that a parallel test server can get.
func unreachableIssuer(t *testing.T) string {
	t.Helper()

	return "http://127.0.0.1:" + testingnet.FreePort(t, "127.0.0.1")
}

func responseCookies(rec *httptest.ResponseRecorder) map[string]string {
	cookies := map[string]string{}
	for _, cookie := range rec.Result().Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	return cookies
}
