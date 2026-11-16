package authn

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_checkContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		body          string
		contentLength int64
		contentType   string
		remoteAddr    string
		tls           *tls.ConnectionState

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success - GET without Content-Type",
			method:        http.MethodGet,
			body:          "",
			contentLength: 0,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - HEAD with text/plain",
			method:        http.MethodHead,
			body:          "",
			contentLength: 0,
			contentType:   "text/plain",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - OPTIONS without Content-Type",
			method:        http.MethodOptions,
			body:          "",
			contentLength: 0,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - POST without body with application/json",
			method:        http.MethodPost,
			body:          "",
			contentLength: 0,
			contentType:   "application/json",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - POST with application/json and parameter",
			method:        http.MethodPost,
			body:          `{}`,
			contentLength: 2,
			contentType:   "application/json; charset=utf-8",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - POST with application/octet-stream",
			method:        http.MethodPost,
			body:          "data",
			contentLength: 4,
			contentType:   "application/octet-stream",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - POST with application/gzip",
			method:        http.MethodPost,
			body:          "data",
			contentLength: 4,
			contentType:   "application/gzip",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - POST without Content-Type through unix socket",
			method:        http.MethodPost,
			body:          `{}`,
			contentLength: 2,
			contentType:   "",
			remoteAddr:    "@",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - PUT without body and without Content-Type",
			method:        http.MethodPut,
			body:          "",
			contentLength: 0,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - PUT with body with application/json",
			method:        http.MethodPut,
			body:          `{}`,
			contentLength: 2,
			contentType:   "application/json",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "success - DELETE without body and without Content-Type",
			method:        http.MethodDelete,
			body:          "",
			contentLength: 0,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.NoError,
		},
		{
			name:          "error - POST without body and without Content-Type",
			method:        http.MethodPost,
			body:          "",
			contentLength: 0,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - POST through TLS with unix socket address",
			method:        http.MethodPost,
			body:          "",
			contentLength: 0,
			contentType:   "",
			remoteAddr:    "@",
			tls:           &tls.ConnectionState{},

			assertErr: require.Error,
		},
		{
			name:          "error - POST with text/plain",
			method:        http.MethodPost,
			body:          `{}`,
			contentLength: 2,
			contentType:   "text/plain;charset=UTF-8",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - POST with form urlencoded",
			method:        http.MethodPost,
			body:          "a=b",
			contentLength: 3,
			contentType:   "application/x-www-form-urlencoded",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - POST with multipart form data",
			method:        http.MethodPost,
			body:          "data",
			contentLength: 4,
			contentType:   "multipart/form-data; boundary=x",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - POST with upper case text/plain",
			method:        http.MethodPost,
			body:          `{}`,
			contentLength: 2,
			contentType:   "Text/Plain",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - POST with malformed Content-Type",
			method:        http.MethodPost,
			body:          `{}`,
			contentLength: 2,
			contentType:   "application/json;;",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - PUT with body without Content-Type",
			method:        http.MethodPut,
			body:          `{}`,
			contentLength: 2,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - PATCH with unknown length without Content-Type",
			method:        http.MethodPatch,
			body:          `{}`,
			contentLength: -1,
			contentType:   "",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
		{
			name:          "error - DELETE without body with text/plain",
			method:        http.MethodDelete,
			body:          "",
			contentLength: 0,
			contentType:   "text/plain",
			remoteAddr:    "192.0.2.1:1234",
			tls:           nil,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			req := httptest.NewRequest(tc.method, "https://oc.example:8443/1.0", strings.NewReader(tc.body))
			req.ContentLength = tc.contentLength
			req.RemoteAddr = tc.remoteAddr
			req.TLS = tc.tls
			req.Header.Set("Content-Type", tc.contentType)

			// Run test
			err := checkContentType(req)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
