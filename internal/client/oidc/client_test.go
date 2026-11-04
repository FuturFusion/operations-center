package oidc_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lxc/incus/v7/test/mini-oidc/minioidc"
	"github.com/lxc/incus/v7/test/mini-oidc/storage"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client/oidc"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

// Naive test for a valid bearer token.
var bearerAccessTokenRe = regexp.MustCompile(`^Bearer [^.]+\.[^.]+\.[^.]+$`)

func TestOIDCClient_Do(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		accessTokenExpiration time.Duration

		wantOIDCContextIsChanged bool
	}{
		{
			name:                  "success - two requests without refresh",
			accessTokenExpiration: 20 * time.Second,

			wantOIDCContextIsChanged: false,
		},
		{
			name:                  "success - two requests with refresh for the second",
			accessTokenExpiration: 10 * time.Second, // The client tries a refresh, if token validity is < 15 seconds.

			wantOIDCContextIsChanged: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			serverAddr := setupMiniOIDC(t, tc.accessTokenExpiration)

			tmpDir := t.TempDir()
			oidcContextFile := filepath.Join(tmpDir, "test-oidc-context.json")

			// The client for Operations Center might pin the certificate of the
			// Operations Center server, so it must never be used to reach the identity
			// provider. Restrict it to the host of Operations Center to catch such a
			// leak instead of silently depending on the identity provider being
			// reachable with the pinned configuration.
			operationsCenterHTTPClient := &http.Client{
				Transport: onlyHostTransport(t, serverAddr),
			}

			oidcClient, err := oidc.NewClient(operationsCenterHTTPClient, oidcContextFile, oidc.WithoutOpenBrowser(), oidc.WithAuthenticateCallback(confirmTokenURL(t)))
			require.NoError(t, err)

			// Run test
			req, err := http.NewRequest(http.MethodGet, serverAddr, http.NoBody)
			require.NoError(t, err)

			resp, err := oidcClient.Do(req)
			require.NoError(t, err)
			err = resp.Body.Close()
			require.NoError(t, err)

			// Assert
			require.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := os.ReadFile(oidcContextFile)
			require.NoError(t, err)

			oidcContext := oidc.OIDCContext{}
			err = json.Unmarshal(body, &oidcContext)
			require.NoError(t, err)

			require.Equal(t, oidcClient.GetOIDCTokens().AccessToken, oidcContext.Tokens.AccessToken)
			require.Equal(t, oidcClient.GetOIDCTokens().RefreshToken, oidcContext.Tokens.RefreshToken)
			require.Equal(t, oidcClient.GetOIDCTokens().Expiry.Truncate(0), oidcContext.Tokens.Expiry.Truncate(0)) // We don't care about the internal monotonic time.
			require.Equal(t, oidcClient.GetOIDCTokens().ExpiresIn, oidcContext.Tokens.ExpiresIn)
			require.Equal(t, oidcClient.GetOIDCTokens().IDToken, oidcContext.Tokens.IDToken)
			require.Equal(t, oidcClient.GetOIDCTokens().TokenType, oidcContext.Tokens.TokenType)

			// Run 2nd request
			resp, err = oidcClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.NoError(t, err)

			// Assert
			require.Equal(t, http.StatusOK, resp.StatusCode)

			body2, err := os.ReadFile(oidcContextFile)
			require.NoError(t, err)

			oidcContext2 := oidc.OIDCContext{}
			err = json.Unmarshal(body2, &oidcContext2)
			require.NoError(t, err)

			require.Equal(t, oidcClient.GetOIDCTokens().AccessToken, oidcContext2.Tokens.AccessToken)
			require.Equal(t, oidcClient.GetOIDCTokens().RefreshToken, oidcContext2.Tokens.RefreshToken)
			require.Equal(t, oidcClient.GetOIDCTokens().Expiry.Truncate(0), oidcContext2.Tokens.Expiry.Truncate(0)) // We don't care about the internal monotonic time.
			require.Equal(t, oidcClient.GetOIDCTokens().ExpiresIn, oidcContext2.Tokens.ExpiresIn)
			require.Equal(t, oidcClient.GetOIDCTokens().IDToken, oidcContext2.Tokens.IDToken)
			require.Equal(t, oidcClient.GetOIDCTokens().TokenType, oidcContext2.Tokens.TokenType)

			// Assert OIDCContext change between 1st and 2nd request.
			equalFunc := require.Equal
			if tc.wantOIDCContextIsChanged {
				equalFunc = require.NotEqual
			}

			equalFunc(t, oidcContext.Tokens.AccessToken, oidcContext2.Tokens.AccessToken)
			equalFunc(t, oidcContext.Tokens.RefreshToken, oidcContext2.Tokens.RefreshToken)
			equalFunc(t, oidcContext.Tokens.Expiry.Truncate(0), oidcContext2.Tokens.Expiry.Truncate(0)) // We don't care about the internal monotonic time.
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setupContextFile func(t *testing.T, oidcContextFile string)

		assertErr      require.ErrorAssertionFunc
		wantTrustTuple oidc.OIDCTrustTuple
	}{
		{
			name: "success - file does not exist",
			setupContextFile: func(t *testing.T, oidcContextFile string) {
				t.Helper()
			},

			assertErr:      require.NoError,
			wantTrustTuple: oidc.OIDCTrustTuple{},
		},
		{
			name: "success - file with trust tuple",
			setupContextFile: func(t *testing.T, oidcContextFile string) {
				t.Helper()

				err := os.WriteFile(oidcContextFile, []byte(`{"trust_tuple":{"issuer":"https://idp.example","client_id":"device","audience":"audience"}}`), 0o600)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			wantTrustTuple: oidc.OIDCTrustTuple{
				Issuer:   "https://idp.example",
				ClientID: "device",
				Audience: "audience",
			},
		},
		{
			name: "error - invalid JSON",
			setupContextFile: func(t *testing.T, oidcContextFile string) {
				t.Helper()

				err := os.WriteFile(oidcContextFile, []byte(`{`), 0o600)
				require.NoError(t, err)
			},

			assertErr:      errassert.Contains("Failed to parse OIDC context file"),
			wantTrustTuple: oidc.OIDCTrustTuple{},
		},
		{
			name: "error - file not readable",
			setupContextFile: func(t *testing.T, oidcContextFile string) {
				t.Helper()

				err := os.Mkdir(oidcContextFile, 0o700)
				require.NoError(t, err)
			},

			assertErr:      errassert.Contains("Failed to read OIDC context file"),
			wantTrustTuple: oidc.OIDCTrustTuple{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			oidcContextFile := filepath.Join(t.TempDir(), "test-oidc-context.json")
			tc.setupContextFile(t, oidcContextFile)

			// Run test
			oidcClient, err := oidc.NewClient(http.DefaultClient, oidcContextFile)

			// Assert
			tc.assertErr(t, err, "unexpected error")
			require.Equal(t, tc.wantTrustTuple, oidc.TrustTupleOf(oidcClient), "unexpected trust tuple")
		})
	}
}

func TestOIDCClient_Do_trustTuple(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler func(issuer string, attackerURL string) http.HandlerFunc

		assertErr      require.ErrorAssertionFunc
		wantStatusCode int
	}{
		{
			name: "error - unauthorized with different issuer",
			handler: func(issuer string, attackerURL string) http.HandlerFunc {
				return trustTupleHandler(attackerURL, "device", "audience", alwaysStatus(http.StatusUnauthorized))
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, oidc.ErrOIDCTrustTupleChanged, a...)
				require.ErrorContains(tt, err, "test-oidc-context.json", a...)
			},
			wantStatusCode: 0,
		},
		{
			name: "error - unauthorized with different client ID",
			handler: func(issuer string, attackerURL string) http.HandlerFunc {
				return trustTupleHandler(issuer, "attacker", "audience", alwaysStatus(http.StatusUnauthorized))
			},

			assertErr:      errassert.Is(oidc.ErrOIDCTrustTupleChanged),
			wantStatusCode: 0,
		},
		{
			name: "error - unauthorized with different audience",
			handler: func(issuer string, attackerURL string) http.HandlerFunc {
				return trustTupleHandler(issuer, "device", "attacker", alwaysStatus(http.StatusUnauthorized))
			},

			assertErr:      errassert.Is(oidc.ErrOIDCTrustTupleChanged),
			wantStatusCode: 0,
		},
		{
			name: "success - ok with different issuer is ignored",
			handler: func(issuer string, attackerURL string) http.HandlerFunc {
				return trustTupleHandler(attackerURL, "device", "audience", alwaysStatus(http.StatusOK))
			},

			assertErr:      require.NoError,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "success - unauthorized with same trust tuple refreshes at stored issuer",
			handler: func(issuer string, attackerURL string) http.HandlerFunc {
				return trustTupleHandler(issuer, "device", "audience", firstStatus(http.StatusUnauthorized, http.StatusOK))
			},

			assertErr:      require.NoError,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "success - unauthorized without trust tuple refreshes at stored issuer",
			handler: func(issuer string, attackerURL string) http.HandlerFunc {
				return trustTupleHandler("", "", "", firstStatus(http.StatusUnauthorized, http.StatusOK))
			},

			assertErr:      require.NoError,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			issuer := setupMiniOIDCIssuer(t, 20*time.Second)

			var attackerHits atomic.Int32
			attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attackerHits.Add(1)
				w.WriteHeader(http.StatusNotFound)
			}))
			t.Cleanup(attacker.Close)

			var handler atomic.Pointer[http.HandlerFunc]
			loginHandler := trustTupleHandler(issuer, "device", "audience", func(r *http.Request) int {
				if !bearerAccessTokenRe.MatchString(r.Header.Get("Authorization")) {
					return http.StatusUnauthorized
				}

				return http.StatusOK
			})
			handler.Store(&loginHandler)

			operationsCenter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				(*handler.Load())(w, r)
			}))
			t.Cleanup(operationsCenter.Close)

			oidcContextFile := filepath.Join(t.TempDir(), "test-oidc-context.json")
			oidcClient, err := oidc.NewClient(http.DefaultClient, oidcContextFile, oidc.WithoutOpenBrowser(), oidc.WithAuthenticateCallback(confirmTokenURL(t)))
			require.NoError(t, err)

			// Login, like during "remote add", to store the trust tuple.
			req, err := http.NewRequest(http.MethodGet, operationsCenter.URL, http.NoBody)
			require.NoError(t, err)

			resp, err := oidcClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, statusCodeOf(t, resp), "login must succeed")

			wantTrustTuple := oidc.OIDCTrustTuple{
				Issuer:   issuer,
				ClientID: "device",
				Audience: "audience",
			}

			require.Equal(t, wantTrustTuple, readOIDCContext(t, oidcContextFile).TrustTuple, "login must store the trust tuple")

			scenarioHandler := tc.handler(issuer, attacker.URL)
			handler.Store(&scenarioHandler)

			// Run test
			resp, err = oidcClient.Do(req)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantStatusCode, statusCodeOf(t, resp), "unexpected status code")
			require.Equal(t, int32(0), attackerHits.Load(), "client must never contact the issuer announced by the response")
			require.Equal(t, wantTrustTuple, readOIDCContext(t, oidcContextFile).TrustTuple, "stored trust tuple must not change")
		})
	}
}

// trustTupleHandler returns a handler, which announces the given trust tuple
// and responds with the status code returned by statusCode.
func trustTupleHandler(issuer string, clientID string, audience string, statusCode func(r *http.Request) int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if issuer != "" {
			w.Header().Set("X-OperationsCenter-OIDC-issuer", issuer)
			w.Header().Set("X-OperationsCenter-OIDC-clientid", clientID)
			w.Header().Set("X-OperationsCenter-OIDC-audience", audience)
		}

		code := statusCode(r)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(http.StatusText(code)))
	}
}

func alwaysStatus(code int) func(r *http.Request) int {
	return func(r *http.Request) int {
		return code
	}
}

// firstStatus returns first for the first request and then for all following requests.
func firstStatus(first int, then int) func(r *http.Request) int {
	var calls atomic.Int32

	return func(r *http.Request) int {
		if calls.Add(1) == 1 {
			return first
		}

		return then
	}
}

// statusCodeOf returns the status code of resp and closes its body. It returns 0 for a nil response.
func statusCodeOf(t *testing.T, resp *http.Response) int {
	t.Helper()

	if resp == nil {
		return 0
	}

	err := resp.Body.Close()
	require.NoError(t, err)

	return resp.StatusCode
}

func readOIDCContext(t *testing.T, oidcContextFile string) oidc.OIDCContext {
	t.Helper()

	body, err := os.ReadFile(oidcContextFile)
	require.NoError(t, err)

	oidcContext := oidc.OIDCContext{}
	err = json.Unmarshal(body, &oidcContext)
	require.NoError(t, err)

	return oidcContext
}

// onlyHostTransport returns a transport, which only performs the requests
// directed at the host of the provided address and fails all the others.
func onlyHostTransport(t *testing.T, addr string) http.RoundTripper {
	t.Helper()

	u, err := url.Parse(addr)
	require.NoError(t, err)

	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != u.Host {
			return nil, fmt.Errorf("unexpected request to %q, this client is only allowed to talk to %q", r.URL.Host, u.Host)
		}

		return http.DefaultTransport.RoundTrip(r)
	})
}

type roundTripperFunc func(r *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func setupMiniOIDC(t *testing.T, accessTokenExpiration time.Duration) string {
	t.Helper()

	issuer := setupMiniOIDCIssuer(t, accessTokenExpiration)
	clientID := "device"

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OperationsCenter-OIDC-issuer", issuer)
		w.Header().Set("X-OperationsCenter-OIDC-clientid", clientID)
		w.Header().Set("X-OperationsCenter-OIDC-audience", "audience")

		if !bearerAccessTokenRe.MatchString(r.Header.Get("Authorization")) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(http.StatusText(http.StatusUnauthorized)))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(http.StatusText(http.StatusOK)))
	}))
	t.Cleanup(httpServer.Close)

	return httpServer.URL
}

// setupMiniOIDCIssuer starts a mini-oidc identity provider and returns its issuer URL.
func setupMiniOIDCIssuer(t *testing.T, accessTokenExpiration time.Duration) string {
	t.Helper()

	issuer := minioidc.RunTest(
		t,
		[]storage.Option{
			storage.WithAccessTokenExpiration(accessTokenExpiration),
		},
		[]minioidc.Option{
			minioidc.WithDeviceAuthorizationPollInterval(1 * time.Second), // smaller than 1 second seems not to work.
		},
	)
	waitForOIDCProviderReady(t, issuer)

	return issuer
}

// confirmTokenURL returns an authenticate callback, which confirms the device
// code with mini-oidc. Mini-oidc issues the tokens when the URL is queried.
func confirmTokenURL(t *testing.T) func(tokenURL string) {
	t.Helper()

	return func(tokenURL string) {
		resp, err := http.Get(tokenURL)
		if err != nil {
			t.Errorf("authenticate callback: %v", err)
			return
		}

		defer func() {
			err := resp.Body.Close()
			if err != nil {
				t.Errorf("authenticate callback: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("authentication callback: unexpected http status: %d", resp.StatusCode)
		}
	}
}

// waitForOIDCProviderReady blocks until the minioidc provider accepts requests.
func waitForOIDCProviderReady(t *testing.T, issuer string) {
	t.Helper()

	const timeout = 10 * time.Second

	discoveryURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"

	var lastErr error

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(discoveryURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}

			lastErr = fmt.Errorf("unexpected http status: %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("minioidc provider not ready after %s: %v", timeout, lastErr)
}
