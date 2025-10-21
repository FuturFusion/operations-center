package oidc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/lxc/incus/v6/test/mini-oidc/minioidc"
	"github.com/lxc/incus/v6/test/mini-oidc/storage"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client/oidc"
)

// Naive test for a valid bearer token.
var bearerAccessTokenRe = regexp.MustCompile(`^Bearer [^.]+\.[^.]+\.[^.]+$`)

func TestOIDCClient_Do(t *testing.T) {
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
			// Setup
			serverAddr := setupMiniOIDC(t, tc.accessTokenExpiration)

			tmpDir := t.TempDir()
			oidcContextFile := filepath.Join(tmpDir, "test-oidc-context.json")

			oidcClient := oidc.NewClient(&http.Client{}, oidcContextFile, oidc.WithoutOpenBrowser(), oidc.WithAuthenticateCallback(func(tokenURL string) {
				resp, err := http.Get(tokenURL)
				if err != nil {
					t.Errorf("authenticate callback: %v", err)
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
			}))

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

func setupMiniOIDC(t *testing.T, accessTokenExpiration time.Duration) string {
	t.Helper()

	issuer := minioidc.RunTest(t,
		[]storage.Option{
			storage.WithAccessTokenExpiration(accessTokenExpiration),
		},
		[]minioidc.Option{
			minioidc.WithDeviceAuthorizationPollInterval(1 * time.Second), // smaller than 1 second seems not to work.
		},
	)
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
