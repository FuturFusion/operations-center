package authn_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authn"
)

type headerAuther struct{}

func (headerAuther) Auth(_ http.ResponseWriter, r *http.Request) (bool, string, string, error) {
	return r.Header.Get("X-Trusted") != "", "user", "test", nil
}

func TestAuthenticator_Middleware_starting(t *testing.T) {
	tests := []struct {
		name      string
		opts      []authn.Option
		trusted   bool
		forwarded bool
		wantCode  int
	}{
		{name: "starting - trusted request", opts: []authn.Option{authn.WithStarting()}, trusted: true, wantCode: http.StatusOK},
		{name: "starting - untrusted request", opts: []authn.Option{authn.WithStarting()}, wantCode: http.StatusTooEarly},
		{name: "starting - forwarded request", opts: []authn.Option{authn.WithStarting()}, trusted: true, forwarded: true, wantCode: http.StatusTooEarly},
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

			if tc.forwarded {
				r.Header.Set(authn.HeaderForwardedAddress, "192.0.2.1:1234")
			}

			w := httptest.NewRecorder()
			handler(w, r)

			require.Equal(t, tc.wantCode, w.Code)
		})
	}
}
