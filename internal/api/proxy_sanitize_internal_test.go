package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewSanitizingProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		requestHeader          http.Header
		upstreamStatusCode     int
		upstreamResponseHeader http.Header

		wantUpstreamRequestHeader http.Header
		wantResponseHeader        http.Header
	}{
		{
			name: "unauthorized with credentials and OIDC headers",
			requestHeader: http.Header{
				"Authorization": []string{"Bearer secret-access-token"},
				"Cookie":        []string{"oidc_access=secret-access-token; oidc_refresh=secret-refresh-token"},
				"X-Other":       []string{"kept"},
			},
			upstreamStatusCode: http.StatusUnauthorized,
			upstreamResponseHeader: http.Header{
				"X-Operationscenter-Oidc-Issuer":   []string{"https://idp.attacker.example"},
				"X-Operationscenter-Oidc-Clientid": []string{"attacker"},
				"X-Operationscenter-Oidc-Audience": []string{"attacker"},
				"X-Other":                          []string{"kept"},
			},

			wantUpstreamRequestHeader: http.Header{
				"X-Other": []string{"kept"},
			},
			wantResponseHeader: http.Header{
				"X-Other": []string{"kept"},
			},
		},
		{
			name: "ok with unknown OIDC header",
			requestHeader: http.Header{
				"X-Other": []string{"kept"},
			},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				"X-Operationscenter-Oidc-Unknown": []string{"attacker"},
				"X-Other":                         []string{"kept"},
			},

			wantUpstreamRequestHeader: http.Header{
				"X-Other": []string{"kept"},
			},
			wantResponseHeader: http.Header{
				"X-Other": []string{"kept"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			upstreamRequestHeaderCh := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamRequestHeaderCh <- r.Header.Clone()

				for name, values := range tc.upstreamResponseHeader {
					w.Header()[name] = values
				}

				w.WriteHeader(tc.upstreamStatusCode)
			}))
			t.Cleanup(upstream.Close)

			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)

			proxyServer := httptest.NewServer(newSanitizingProxy(http.DefaultTransport, upstreamURL.Scheme, upstreamURL.Host))
			t.Cleanup(proxyServer.Close)

			req, err := http.NewRequest(http.MethodGet, proxyServer.URL, http.NoBody)
			require.NoError(t, err)

			req.Header = tc.requestHeader

			// Run test
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			var upstreamRequestHeader http.Header
			select {
			case upstreamRequestHeader = <-upstreamRequestHeaderCh:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "upstream did not receive the request")
			}

			// Assert
			require.Equal(t, tc.upstreamStatusCode, resp.StatusCode, "status code from upstream is passed through")
			require.Equal(t, tc.wantUpstreamRequestHeader, filterHeader(upstreamRequestHeader, tc.requestHeader), "upstream must not receive the credentials")
			require.Equal(t, tc.wantResponseHeader, filterHeader(resp.Header, tc.upstreamResponseHeader), "proxy must remove the OIDC headers from the response")
		})
	}
}

// filterHeader returns the headers of header with a name present in names.
// This ignores the headers added by the HTTP stack.
func filterHeader(header http.Header, names http.Header) http.Header {
	filtered := http.Header{}
	for name := range names {
		values := header.Values(name)
		if len(values) > 0 {
			filtered[http.CanonicalHeaderKey(name)] = values
		}
	}

	return filtered
}
