package api

import (
	"io"
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
		upstreamBody           string

		wantUpstreamRequestHeader http.Header
		wantResponseHeader        http.Header
		wantBody                  string
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
				"Content-Type":                     []string{"application/json"},
				"X-Operationscenter-Oidc-Issuer":   []string{"https://idp.attacker.example"},
				"X-Operationscenter-Oidc-Clientid": []string{"attacker"},
				"X-Operationscenter-Oidc-Audience": []string{"attacker"},
				"X-Other":                          []string{"kept"},
			},
			upstreamBody: `{"error":"unauthorized"}`,

			wantUpstreamRequestHeader: http.Header{
				"X-Other": []string{"kept"},
			},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/json"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
				"X-Other":                 []string{"kept"},
			},
			wantBody: `{"error":"unauthorized"}`,
		},
		{
			name: "ok with unknown OIDC header",
			requestHeader: http.Header{
				"X-Other": []string{"kept"},
			},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				"Content-Type":                    []string{"application/json"},
				"X-Operationscenter-Oidc-Unknown": []string{"attacker"},
				"X-Other":                         []string{"kept"},
			},
			upstreamBody: `{}`,

			wantUpstreamRequestHeader: http.Header{
				"X-Other": []string{"kept"},
			},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/json"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
				"X-Other":                 []string{"kept"},
			},
			wantBody: `{}`,
		},
		{
			name:               "html is replaced with octet-stream",
			requestHeader:      http.Header{},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			upstreamBody: `<script>alert(document.cookie)</script>`,

			wantUpstreamRequestHeader: http.Header{},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/octet-stream"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
			},
			wantBody: `<script>alert(document.cookie)</script>`,
		},
		{
			name:               "text/plain with parameters is kept",
			requestHeader:      http.Header{},
			upstreamStatusCode: http.StatusNotFound,
			upstreamResponseHeader: http.Header{
				"Content-Type": []string{"text/plain; charset=utf-8"},
			},
			upstreamBody: "not found",

			wantUpstreamRequestHeader: http.Header{},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"text/plain; charset=utf-8"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
			},
			wantBody: "not found",
		},
		{
			name:               "gzip is kept",
			requestHeader:      http.Header{},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				"Content-Type": []string{"application/gzip"},
			},
			upstreamBody: "backup",

			wantUpstreamRequestHeader: http.Header{},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/gzip"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
			},
			wantBody: "backup",
		},
		{
			name:               "missing content type is replaced with octet-stream",
			requestHeader:      http.Header{},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				// A nil value prevents content sniffing by the upstream server.
				"Content-Type": nil,
			},
			upstreamBody: `<html><script>alert(1)</script></html>`,

			wantUpstreamRequestHeader: http.Header{},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/octet-stream"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
			},
			wantBody: `<html><script>alert(1)</script></html>`,
		},
		{
			name:               "invalid content type is replaced with octet-stream",
			requestHeader:      http.Header{},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				"Content-Type": []string{"application/json; ="},
			},
			upstreamBody: `{}`,

			wantUpstreamRequestHeader: http.Header{},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/octet-stream"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
			},
			wantBody: `{}`,
		},
		{
			name:               "cookie and security headers from upstream are replaced or removed",
			requestHeader:      http.Header{},
			upstreamStatusCode: http.StatusOK,
			upstreamResponseHeader: http.Header{
				"Content-Type":            []string{"application/json"},
				"Set-Cookie":              []string{"oidc_access=attacker; Path=/"},
				"Clear-Site-Data":         []string{`"cookies"`},
				"X-Content-Type-Options":  []string{"none"},
				"Content-Security-Policy": []string{"default-src *"},
			},
			upstreamBody: `{}`,

			wantUpstreamRequestHeader: http.Header{},
			wantResponseHeader: http.Header{
				"Content-Type":            []string{"application/json"},
				"X-Content-Type-Options":  []string{"nosniff"},
				"Content-Security-Policy": []string{proxyResponseContentSecurityPolicy},
			},
			wantBody: `{}`,
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
				_, _ = w.Write([]byte(tc.upstreamBody))
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

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var upstreamRequestHeader http.Header
			select {
			case upstreamRequestHeader = <-upstreamRequestHeaderCh:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "upstream did not receive the request")
			}

			// Assert
			require.Equal(t, tc.upstreamStatusCode, resp.StatusCode, "status code from upstream is passed through")
			require.Equal(t, tc.wantUpstreamRequestHeader, filterHeader(upstreamRequestHeader, tc.requestHeader), "upstream must not receive the credentials")
			require.Equal(t, tc.wantResponseHeader, filterHeader(resp.Header, tc.upstreamResponseHeader, sanitizedResponseHeaderNames), "proxy must sanitize the response headers")
			require.Equal(t, tc.wantBody, string(body), "body from upstream is passed through")
		})
	}
}

// sanitizedResponseHeaderNames are the response headers the proxy sets or removes.
var sanitizedResponseHeaderNames = http.Header{
	"Content-Type":            nil,
	"X-Content-Type-Options":  nil,
	"Content-Security-Policy": nil,
	"Set-Cookie":              nil,
	"Clear-Site-Data":         nil,
}

// filterHeader returns the headers of header with a name present in one of names.
// This ignores the headers added by the HTTP stack.
func filterHeader(header http.Header, names ...http.Header) http.Header {
	filtered := http.Header{}
	for _, n := range names {
		for name := range n {
			values := header.Values(name)
			if len(values) > 0 {
				filtered[http.CanonicalHeaderKey(name)] = values
			}
		}
	}

	return filtered
}
