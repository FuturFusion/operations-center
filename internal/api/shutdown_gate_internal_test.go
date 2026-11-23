package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_shutdownGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		shuttingDown bool
		method       string
		path         string

		wantStatus int
	}{
		{
			name:   "change while running",
			method: http.MethodPost,
			path:   "/1.0/provisioning/clusters",

			wantStatus: http.StatusOK,
		},
		{
			name:         "change while shutting down",
			shuttingDown: true,
			method:       http.MethodPost,
			path:         "/1.0/provisioning/clusters",

			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:         "read while shutting down",
			shuttingDown: true,
			method:       http.MethodGet,
			path:         "/1.0/provisioning/clusters",

			wantStatus: http.StatusOK,
		},
		{
			name:         "internal API while shutting down",
			shuttingDown: true,
			method:       http.MethodPost,
			path:         "/internal/membership/:assign",

			wantStatus: http.StatusOK,
		},
		{
			name:         "IncusOS API while shutting down",
			shuttingDown: true,
			method:       http.MethodPost,
			path:         "/os/1.0/system/:reboot",

			wantStatus: http.StatusOK,
		},
		{
			name:         "path only starting like the internal API while shutting down",
			shuttingDown: true,
			method:       http.MethodDelete,
			path:         "/internalfoo",

			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var shuttingDown atomic.Bool
			shuttingDown.Store(tc.shuttingDown)

			handler := shutdownGate(&shuttingDown, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, http.NoBody))

			require.Equal(t, tc.wantStatus, rec.Code)
		})
	}
}
