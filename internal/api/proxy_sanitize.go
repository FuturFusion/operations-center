package api

import (
	"log/slog"
	"mime"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// oidcResponseHeaderPrefix is the canonical prefix of the OIDC trust tuple headers.
const oidcResponseHeaderPrefix = "X-Operationscenter-Oidc-"

// proxyResponseContentSecurityPolicy prevents the browser from running script
// from a proxied response on the Operations Center origin.
const proxyResponseContentSecurityPolicy = "sandbox; default-src 'none'; frame-ancestors 'none'"

// proxyResponseAllowedMediaTypes are the media types IncusOS returns. The proxy
// replaces any other media type with application/octet-stream.
var proxyResponseAllowedMediaTypes = map[string]struct{}{
	"application/json":         {},
	"application/gzip":         {},
	"application/x-tar":        {},
	"application/octet-stream": {},
	"text/plain":               {},
}

// newSanitizingProxy returns a reverse proxy to IncusOS. It removes the
// credentials of the Operations Center user from the requests. It hardens the
// responses, so the proxy target cannot run script or set cookies on the
// Operations Center origin.
func newSanitizingProxy(transport http.RoundTripper, scheme string, host string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = scheme
			r.Out.URL.Host = host
			stripClientCredentials(r)
		},
		ModifyResponse: sanitizeResponse,
	}
}

// stripClientCredentials removes the credentials of the Operations Center user
// from a proxied request. The proxy target must not receive the tokens of the user.
func stripClientCredentials(r *httputil.ProxyRequest) {
	r.Out.Header.Del("Authorization")
	r.Out.Header.Del("Cookie")
}

// sanitizeResponse hardens a proxied response. The proxy target is not trusted,
// but the browser treats the response as content of the Operations Center origin.
func sanitizeResponse(resp *http.Response) error {
	// Only Operations Center itself may tell the client which OIDC issuer to use.
	for name := range resp.Header {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), oidcResponseHeaderPrefix) {
			resp.Header.Del(name)
		}
	}

	resp.Header.Del("Set-Cookie")
	resp.Header.Del("Clear-Site-Data")

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	_, allowed := proxyResponseAllowedMediaTypes[mediaType]
	if err != nil || !allowed {
		slog.WarnContext(resp.Request.Context(), "Replace content type of proxied response",
			slog.String("content_type", contentType),
			slog.String("host", resp.Request.URL.Host),
			slog.String("path", resp.Request.URL.Path),
			logger.Err(err),
		)

		resp.Header.Set("Content-Type", "application/octet-stream")
	}

	resp.Header.Set("X-Content-Type-Options", "nosniff")
	resp.Header.Set("Content-Security-Policy", proxyResponseContentSecurityPolicy)

	return nil
}
