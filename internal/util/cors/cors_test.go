package cors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/cors"
)

func TestHandler(t *testing.T) {
	t.Parallel()

	fullConfig := cors.CORSConfig{
		AllowedOrigins:   "https://example.com",
		AllowedMethods:   "GET, POST",
		AllowedHeaders:   "Content-Type",
		AllowCredentials: true,
	}

	tests := []struct {
		name   string
		config cors.CORSConfig
		origin string

		wantHeader http.Header
	}{
		{
			name:   "full config with origin",
			config: fullConfig,
			origin: "https://example.com",

			wantHeader: http.Header{
				"Access-Control-Allow-Origin":      {"https://example.com"},
				"Access-Control-Allow-Methods":     {"GET, POST"},
				"Access-Control-Allow-Headers":     {"Content-Type"},
				"Access-Control-Allow-Credentials": {"true"},
			},
		},
		{
			name:   "full config without origin",
			config: fullConfig,
			origin: "",

			wantHeader: http.Header{
				"Access-Control-Allow-Credentials": {"true"},
			},
		},
		{
			name:   "empty config with origin",
			config: cors.CORSConfig{},
			origin: "https://example.com",

			wantHeader: http.Header{},
		},
		{
			name: "only allowed origins",
			config: cors.CORSConfig{
				AllowedOrigins: "*",
			},
			origin: "https://example.com",

			wantHeader: http.Header{
				"Access-Control-Allow-Origin": {"*"},
			},
		},
		{
			name: "only allowed methods",
			config: cors.CORSConfig{
				AllowedMethods: "GET",
			},
			origin: "https://example.com",

			wantHeader: http.Header{
				"Access-Control-Allow-Methods": {"GET"},
			},
		},
		{
			name: "only allowed headers",
			config: cors.CORSConfig{
				AllowedHeaders: "Authorization",
			},
			origin: "https://example.com",

			wantHeader: http.Header{
				"Access-Control-Allow-Headers": {"Authorization"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			nextCalled := false
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
			}

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()

			// Run test
			cors.Handler(tc.config)(next)(rec, req)

			// Assert
			require.Equal(t, tc.wantHeader, rec.Header(), "response headers")
			require.True(t, nextCalled, "next handler must be called")
			require.Empty(t, req.Header.Get("Access-Control-Allow-Credentials"), "credentials header not set on the request")
		})
	}
}
