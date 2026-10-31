package authn_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authn"
)

type trustedAuther struct{}

func (trustedAuther) Auth(w http.ResponseWriter, r *http.Request) (bool, string, string, error) {
	return true, "testuser", "testprotocol", nil
}

func TestAuthenticator_Middleware_crossOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		headers map[string]string

		wantStatusCode int
	}{
		{
			name:   "success - POST without browser headers",
			method: http.MethodPost,

			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "success - POST with Sec-Fetch-Site same-origin",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "same-origin",
			},

			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "success - POST with Sec-Fetch-Site none",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "none",
			},

			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "success - POST with Origin matching Host",
			method: http.MethodPost,
			headers: map[string]string{
				"Origin": "https://oc.example:8443",
			},

			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "success - GET with Sec-Fetch-Site cross-site",
			method: http.MethodGet,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://attacker.example",
			},

			wantStatusCode: http.StatusNoContent,
		},
		{
			name:   "error - POST with Sec-Fetch-Site cross-site",
			method: http.MethodPost,
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://attacker.example",
				"Content-Type":   "text/plain",
			},

			wantStatusCode: http.StatusForbidden,
		},
		{
			name:   "error - PUT with Sec-Fetch-Site same-site",
			method: http.MethodPut,
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Origin":         "https://other.oc.example",
			},

			wantStatusCode: http.StatusForbidden,
		},
		{
			name:   "error - DELETE with foreign Origin and without Sec-Fetch-Site",
			method: http.MethodDelete,
			headers: map[string]string{
				"Origin": "https://attacker.example",
			},

			wantStatusCode: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			authenticator := authn.New([]authn.Auther{trustedAuther{}})
			handler := authenticator.Middleware()(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(tc.method, "https://oc.example:8443/internal/sql", http.NoBody)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			rec := httptest.NewRecorder()

			// Run test
			handler(rec, req)

			// Assert
			require.Equal(t, tc.wantStatusCode, rec.Code, "unexpected status code, body: %s", rec.Body.String())
		})
	}
}

type headerAuther struct{}

func (headerAuther) Auth(_ http.ResponseWriter, r *http.Request) (bool, string, string, error) {
	return r.Header.Get("X-Trusted") != "", "user", "test", nil
}

func TestAuthenticator_Middleware_starting(t *testing.T) {
	tests := []struct {
		name     string
		opts     []authn.Option
		trusted  bool
		wantCode int
	}{
		{name: "starting - trusted request", opts: []authn.Option{authn.WithStarting()}, trusted: true, wantCode: http.StatusOK},
		{name: "starting - untrusted request", opts: []authn.Option{authn.WithStarting()}, wantCode: http.StatusTooEarly},
		{name: "started - untrusted request", wantCode: http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authenticator := authn.New([]authn.Auther{headerAuther{}}, tc.opts...)
			handler := authenticator.Middleware()(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			r := httptest.NewRequest(http.MethodGet, "/1.0", nil)
			if tc.trusted {
				r.Header.Set("X-Trusted", "1")
			}

			w := httptest.NewRecorder()
			handler(w, r)

			require.Equal(t, tc.wantCode, w.Code)
		})
	}
}
