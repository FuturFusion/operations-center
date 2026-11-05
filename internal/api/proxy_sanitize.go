package api

import (
	"net/http"
	"net/http/httputil"
	"strings"
)

// oidcResponseHeaderPrefix is the canonical prefix of the OIDC trust tuple headers.
const oidcResponseHeaderPrefix = "X-Operationscenter-Oidc-"

// newSanitizingProxy returns a reverse proxy to IncusOS. It removes the
// credentials of the Operations Center user from the requests and the OIDC
// trust tuple headers from the responses.
func newSanitizingProxy(transport http.RoundTripper, scheme string, host string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = scheme
			r.Out.URL.Host = host
			stripClientCredentials(r)
		},
		ModifyResponse: stripOIDCHeaders,
	}
}

// stripClientCredentials removes the credentials of the Operations Center user
// from a proxied request. The proxy target must not receive the tokens of the user.
func stripClientCredentials(r *httputil.ProxyRequest) {
	r.Out.Header.Del("Authorization")
	r.Out.Header.Del("Cookie")
}

// stripOIDCHeaders removes the OIDC trust tuple headers from a proxied response.
// Only Operations Center itself may tell the client which OIDC issuer to use.
func stripOIDCHeaders(resp *http.Response) error {
	for name := range resp.Header {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), oidcResponseHeaderPrefix) {
			resp.Header.Del(name)
		}
	}

	return nil
}
