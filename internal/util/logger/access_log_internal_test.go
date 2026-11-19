package logger

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccessLogMiddleware_ResponseController(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(rc *http.ResponseController) error

		wantFlushed  bool
		wantHijacked bool
	}{
		{
			name: "flush",
			call: (*http.ResponseController).Flush,

			wantFlushed:  true,
			wantHijacked: false,
		},
		{
			name: "hijack",
			call: func(rc *http.ResponseController) error {
				_, _, err := rc.Hijack()
				return err
			},

			wantFlushed:  false,
			wantHijacked: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			w := &controllerWriter{ResponseWriter: httptest.NewRecorder()}

			var err error
			handler := AccessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err = tc.call(http.NewResponseController(w))
			}))

			// Run test
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

			// Assert
			require.NoError(t, err, "the response controller reaches the wrapped writer")
			require.Equal(t, tc.wantFlushed, w.flushed, "the wrapped writer is flushed")
			require.Equal(t, tc.wantHijacked, w.hijacked, "the wrapped writer is hijacked")
		})
	}
}

type controllerWriter struct {
	http.ResponseWriter

	flushed  bool
	hijacked bool
}

func (w *controllerWriter) Flush() {
	w.flushed = true
}

func (w *controllerWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true

	return nil, nil, nil
}
