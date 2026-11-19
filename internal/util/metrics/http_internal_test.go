package metrics

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

var errCrash = errors.New("crash")

// TestHTTPMiddleware, TestHTTPMiddleware_server and TestHTTPMiddleware_gathered
// do not use t.Parallel.
// They change the global metrics and the enabled components.

func TestHTTPMiddleware(t *testing.T) {
	serve := func(method string, target string) func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
		return func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
			t.Helper()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))

			return rec
		}
	}

	servePanic := func(target string, wantPanic any) func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
		return func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
			t.Helper()

			rec := httptest.NewRecorder()
			require.PanicsWithValue(t, wantPanic, func() {
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
			}, "the middleware panics again with the same value")

			return rec
		}
	}

	serveHijackable := func(target string) func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
		return func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
			t.Helper()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(&controllerWriter{ResponseWriter: rec}, httptest.NewRequest(http.MethodGet, target, nil))

			return rec
		}
	}

	serveHijackablePanic := func(target string, wantPanic any) func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
		return func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
			t.Helper()

			rec := httptest.NewRecorder()
			require.PanicsWithValue(t, wantPanic, func() {
				handler.ServeHTTP(&controllerWriter{ResponseWriter: rec}, httptest.NewRequest(http.MethodGet, target, nil))
			}, "the middleware panics again with the same value")

			return rec
		}
	}

	tests := []struct {
		name              string
		pattern           string
		handler           http.HandlerFunc
		enabledComponents []string
		request           func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder

		wantStatusCode     int
		wantBody           string
		wantMethod         string
		wantRoute          string
		wantCode           string
		wantSeries         int
		wantDurationSeries int
		wantErrorSeries    int
		wantAbortSeries    int
		wantRequests       float64
		wantErrors         float64
		wantAborted        float64
		wantObservations   uint64
	}{
		{
			name:              "success - matched route",
			pattern:           "GET /items/{id}",
			handler:           writeBody("item"),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/items/42"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "item",
			wantMethod:         http.MethodGet,
			wantRoute:          "/items/{id}",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - response without explicit status code",
			pattern:           "GET /empty",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/empty"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/empty",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - body without explicit status code",
			pattern:           "GET /write",
			handler:           writeBody("body"),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/write"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "body",
			wantMethod:         http.MethodGet,
			wantRoute:          "/write",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - pattern without method",
			pattern:           "/proxy/{name}/",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/proxy/one/path"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/proxy/{name}/",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - redirect to the clean path",
			pattern:           "GET /items/{id}",
			handler:           writeBody("item"),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/items//42"),

			wantStatusCode:     http.StatusTemporaryRedirect,
			wantBody:           "<a href=\"/items/42\">Temporary Redirect</a>.\n\n",
			wantMethod:         http.MethodGet,
			wantRoute:          "/items/{id}",
			wantCode:           "307",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - unmatched route",
			pattern:           "GET /items/{id}",
			handler:           writeBody("item"),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/does/not/exist"),

			wantStatusCode:     http.StatusNotFound,
			wantBody:           "404 page not found\n",
			wantMethod:         http.MethodGet,
			wantRoute:          routeUnmatched,
			wantCode:           "404",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - unknown method",
			pattern:           "GET /items/{id}",
			handler:           writeBody("item"),
			enabledComponents: []string{"api"},
			request:           serve("BREW", "/does/not/exist"),

			wantStatusCode:     http.StatusNotFound,
			wantBody:           "404 page not found\n",
			wantMethod:         methodOther,
			wantRoute:          routeUnmatched,
			wantCode:           "404",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - method not allowed",
			pattern:           "GET /items/{id}",
			handler:           writeBody("item"),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodPost, "/items/42"),

			wantStatusCode:     http.StatusMethodNotAllowed,
			wantBody:           "Method Not Allowed\n",
			wantMethod:         http.MethodPost,
			wantRoute:          routeUnmatched,
			wantCode:           "405",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - informational status code before the final one",
			pattern:           "GET /early-hints",
			handler:           writeStatus(http.StatusEarlyHints, http.StatusOK),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/early-hints"),

			wantStatusCode:     http.StatusEarlyHints,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/early-hints",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "aborted - response aborts after the body started",
			pattern:           "GET /write-then-panic",
			handler:           writeBodyThenPanic("body", http.ErrAbortHandler),
			enabledComponents: []string{"api"},
			request:           servePanic("/write-then-panic", http.ErrAbortHandler),

			wantStatusCode:     http.StatusOK,
			wantBody:           "body",
			wantMethod:         http.MethodGet,
			wantRoute:          "/write-then-panic",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    1,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        1,
			wantObservations:   1,
		},
		{
			name:              "aborted - response aborts after the status code",
			pattern:           "GET /created-then-panic",
			handler:           writeStatusThenPanic(http.StatusCreated, http.ErrAbortHandler),
			enabledComponents: []string{"api"},
			request:           servePanic("/created-then-panic", http.ErrAbortHandler),

			wantStatusCode:     http.StatusCreated,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/created-then-panic",
			wantCode:           "201",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    1,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        1,
			wantObservations:   1,
		},
		{
			name:              "error - status 500",
			pattern:           "GET /fail",
			handler:           writeStatus(http.StatusInternalServerError),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/fail"),

			wantStatusCode:     http.StatusInternalServerError,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/fail",
			wantCode:           "500",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - status 503",
			pattern:           "GET /unavailable",
			handler:           writeStatus(http.StatusServiceUnavailable),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/unavailable"),

			wantStatusCode:     http.StatusServiceUnavailable,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/unavailable",
			wantCode:           "503",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - informational status code before status 500",
			pattern:           "GET /early-hints-then-fail",
			handler:           writeStatus(http.StatusEarlyHints, http.StatusInternalServerError),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/early-hints-then-fail"),

			wantStatusCode:     http.StatusEarlyHints,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/early-hints-then-fail",
			wantCode:           "500",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - first status code wins",
			pattern:           "GET /fail-twice",
			handler:           writeStatus(http.StatusInternalServerError, http.StatusOK),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/fail-twice"),

			wantStatusCode:     http.StatusInternalServerError,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/fail-twice",
			wantCode:           "500",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - handler aborts before the response",
			pattern:           "GET /panic",
			handler:           panicWith(http.ErrAbortHandler),
			enabledComponents: []string{"api"},
			request:           servePanic("/panic", http.ErrAbortHandler),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/panic",
			wantCode:           "500",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - handler crashes before the response",
			pattern:           "GET /crash",
			handler:           panicWith(errCrash),
			enabledComponents: []string{"api"},
			request:           servePanic("/crash", errCrash),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/crash",
			wantCode:           "500",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - handler crashes after the body started",
			pattern:           "GET /write-then-crash",
			handler:           writeBodyThenPanic("body", errCrash),
			enabledComponents: []string{"api"},
			request:           servePanic("/write-then-crash", errCrash),

			wantStatusCode:     http.StatusOK,
			wantBody:           "body",
			wantMethod:         http.MethodGet,
			wantRoute:          "/write-then-crash",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    1,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        1,
			wantObservations:   1,
		},
		{
			name:              "success - unknown method on a matched route",
			pattern:           "/proxy/{name}/",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve("BREW", "/proxy/one/path"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         methodOther,
			wantRoute:          "/proxy/{name}/",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - lowercase method",
			pattern:           "/proxy/{name}/",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve("get", "/proxy/one/path"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         methodOther,
			wantRoute:          "/proxy/{name}/",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - pattern with a tab after the method",
			pattern:           "GET\t/tab",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/tab"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/tab",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - pattern with several spaces after the method",
			pattern:           "GET  /spaces",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/spaces"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/spaces",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - pattern with a host",
			pattern:           "GET example.com/host",
			handler:           noop,
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "http://example.com/host"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "example.com/host",
			wantCode:           "200",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - switching protocols",
			pattern:           "GET /switch",
			handler:           writeStatus(http.StatusSwitchingProtocols),
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/switch"),

			wantStatusCode:     http.StatusSwitchingProtocols,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/switch",
			wantCode:           "101",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "success - hijacked connection",
			pattern:           "GET /hijack",
			handler:           hijack,
			enabledComponents: []string{"api"},
			request:           serveHijackable("/hijack"),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/hijack",
			wantCode:           "101",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - hijack not supported",
			pattern:           "GET /hijack",
			handler:           hijack,
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/hijack"),

			wantStatusCode:     http.StatusBadGateway,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/hijack",
			wantCode:           "502",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    0,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        0,
			wantObservations:   1,
		},
		{
			name:              "error - handler crashes after the hijack",
			pattern:           "GET /hijack-then-crash",
			handler:           hijackThenPanic(errCrash),
			enabledComponents: []string{"api"},
			request:           serveHijackablePanic("/hijack-then-crash", errCrash),

			wantStatusCode:     http.StatusOK,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/hijack-then-crash",
			wantCode:           "101",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    1,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        1,
			wantObservations:   1,
		},
		{
			name:              "aborted - response aborts after status 500",
			pattern:           "GET /fail-then-panic",
			handler:           writeStatusThenPanic(http.StatusInternalServerError, http.ErrAbortHandler),
			enabledComponents: []string{"api"},
			request:           servePanic("/fail-then-panic", http.ErrAbortHandler),

			wantStatusCode:     http.StatusInternalServerError,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/fail-then-panic",
			wantCode:           "500",
			wantSeries:         1,
			wantDurationSeries: 1,
			wantErrorSeries:    1,
			wantAbortSeries:    1,
			wantRequests:       1,
			wantErrors:         1,
			wantAborted:        1,
			wantObservations:   1,
		},
		{
			name:              "disabled - component disabled",
			pattern:           "GET /fail",
			handler:           writeStatus(http.StatusInternalServerError),
			enabledComponents: []string{"runtime"},
			request:           serve(http.MethodGet, "/fail"),

			wantStatusCode:     http.StatusInternalServerError,
			wantBody:           "",
			wantMethod:         http.MethodGet,
			wantRoute:          "/fail",
			wantCode:           "500",
			wantSeries:         0,
			wantDurationSeries: 0,
			wantErrorSeries:    0,
			wantAbortSeries:    0,
			wantRequests:       0,
			wantErrors:         0,
			wantAborted:        0,
			wantObservations:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			httpRequestsTotal.Reset()
			httpRequestErrorsTotal.Reset()
			httpRequestsAbortedTotal.Reset()
			httpRequestDuration.Reset()

			SetEnabledComponents(tc.enabledComponents)
			t.Cleanup(func() { SetEnabledComponents(nil) })

			mux := http.NewServeMux()
			mux.HandleFunc(tc.pattern, tc.handler)

			// Run test
			rec := tc.request(t, HTTPMiddleware(mux))

			// Assert
			// Count the series before the WithLabelValues calls below. These
			// calls create the series that they read.
			require.Equal(t, tc.wantStatusCode, rec.Code, "status code of the response. httptest.ResponseRecorder keeps the first status code")
			require.Equal(t, tc.wantBody, rec.Body.String(), "body of the response")
			require.Equal(t, tc.wantSeries, testutil.CollectAndCount(httpRequestsTotal), "number of request counter series")
			require.Equal(t, tc.wantErrorSeries, testutil.CollectAndCount(httpRequestErrorsTotal), "number of error counter series")
			require.Equal(t, tc.wantAbortSeries, testutil.CollectAndCount(httpRequestsAbortedTotal), "number of aborted counter series")
			require.Equal(t, tc.wantDurationSeries, testutil.CollectAndCount(httpRequestDuration), "number of duration histogram series")
			require.InDelta(t, tc.wantRequests, testutil.ToFloat64(httpRequestsTotal.WithLabelValues(tc.wantMethod, tc.wantRoute, tc.wantCode)), 0, "request counter")
			require.InDelta(t, tc.wantErrors, testutil.ToFloat64(httpRequestErrorsTotal.WithLabelValues(tc.wantMethod, tc.wantRoute)), 0, "error counter")
			require.InDelta(t, tc.wantAborted, testutil.ToFloat64(httpRequestsAbortedTotal.WithLabelValues(tc.wantMethod, tc.wantRoute)), 0, "aborted counter")
			require.Equal(t, tc.wantObservations, observations(t, tc.wantMethod, tc.wantRoute), "number of duration observations")
		})
	}
}

func TestHTTPMiddleware_server(t *testing.T) {
	upgradeHeader := http.Header{
		"Connection": []string{"Upgrade"},
		"Upgrade":    []string{"test"},
	}

	tests := []struct {
		name    string
		handler func(t *testing.T) http.Handler
		header  http.Header

		assertErr      require.ErrorAssertionFunc
		wantStatusCode int
		wantCode       string
		wantErrors     float64
		wantAborted    float64
	}{
		{
			name: "error - handler aborts before the response",
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				return panicWith(http.ErrAbortHandler)
			},
			header: http.Header{},

			assertErr:      require.Error,
			wantStatusCode: 0,
			wantCode:       "500",
			wantErrors:     1,
			wantAborted:    0,
		},
		{
			name: "error - handler crashes before the response",
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				return panicWith(errCrash)
			},
			header: http.Header{},

			assertErr:      require.Error,
			wantStatusCode: 0,
			wantCode:       "500",
			wantErrors:     1,
			wantAborted:    0,
		},
		{
			name: "success - informational status code before the final one",
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				return writeStatus(http.StatusEarlyHints, http.StatusOK)
			},
			header: http.Header{},

			assertErr:      require.NoError,
			wantStatusCode: http.StatusOK,
			wantCode:       "200",
			wantErrors:     0,
			wantAborted:    0,
		},
		{
			name: "success - protocol upgrade through a reverse proxy",
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, brw, err := http.NewResponseController(w).Hijack()
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}

					defer func() { _ = conn.Close() }()

					_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
					_ = brw.Flush()
				}))
				t.Cleanup(backend.Close)

				backendURL, err := url.Parse(backend.URL)
				require.NoError(t, err, "parse the backend URL")

				return httputil.NewSingleHostReverseProxy(backendURL)
			},
			header: upgradeHeader,

			assertErr:      require.NoError,
			wantStatusCode: http.StatusSwitchingProtocols,
			wantCode:       "101",
			wantErrors:     0,
			wantAborted:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			httpRequestsTotal.Reset()
			httpRequestErrorsTotal.Reset()
			httpRequestsAbortedTotal.Reset()
			httpRequestDuration.Reset()

			SetEnabledComponents([]string{"api"})
			t.Cleanup(func() { SetEnabledComponents(nil) })

			mux := http.NewServeMux()
			mux.Handle("GET /path", tc.handler(t))

			// The deferred close runs after the middleware recorded the metrics.
			done := make(chan struct{})
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)

				HTTPMiddleware(mux).ServeHTTP(w, r)
			}))
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.Start()
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/path", http.NoBody)
			require.NoError(t, err, "create the request")

			req.Header = tc.header

			// Run test
			resp, err := server.Client().Do(req)
			closeBody(resp)

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "the handler did not return")
			}

			// Assert
			tc.assertErr(t, err, "the client receives a response only if the handler started it")
			require.Equal(t, tc.wantStatusCode, statusCode(resp), "status code the client receives")
			require.InDelta(t, 1, testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodGet, "/path", tc.wantCode)), 0, "request counter")
			require.InDelta(t, tc.wantErrors, testutil.ToFloat64(httpRequestErrorsTotal.WithLabelValues(http.MethodGet, "/path")), 0, "error counter")
			require.InDelta(t, tc.wantAborted, testutil.ToFloat64(httpRequestsAbortedTotal.WithLabelValues(http.MethodGet, "/path")), 0, "aborted counter")
		})
	}
}

func closeBody(resp *http.Response) {
	if resp != nil {
		_ = resp.Body.Close()
	}
}

func statusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}

	return resp.StatusCode
}

func TestHTTPMiddleware_gathered(t *testing.T) {
	tests := []struct {
		name              string
		enabledComponents []string

		wantCount int
	}{
		{
			name:              "component enabled",
			enabledComponents: []string{"api"},

			wantCount: 4,
		},
		{
			name:              "component disabled",
			enabledComponents: nil,

			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			httpRequestsTotal.Reset()
			httpRequestErrorsTotal.Reset()
			httpRequestsAbortedTotal.Reset()
			httpRequestDuration.Reset()

			httpRequestsTotal.WithLabelValues(http.MethodGet, "/fail", "500").Inc()
			httpRequestErrorsTotal.WithLabelValues(http.MethodGet, "/fail").Inc()
			httpRequestsAbortedTotal.WithLabelValues(http.MethodGet, "/fail").Inc()
			httpRequestDuration.WithLabelValues(http.MethodGet, "/fail").Observe(1)

			SetEnabledComponents(tc.enabledComponents)
			t.Cleanup(func() { SetEnabledComponents(nil) })

			// Run test
			count, err := testutil.GatherAndCount(
				Gatherer(),
				"operations_center_http_requests_total",
				"operations_center_http_request_errors_total",
				"operations_center_http_requests_aborted_total",
				"operations_center_http_request_duration_seconds",
			)

			// Assert
			require.NoError(t, err, "gather the HTTP metrics")
			require.Equal(t, tc.wantCount, count, "number of exposed HTTP series")
		})
	}
}

func TestStatusRecorder_ResponseController(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(rc *http.ResponseController) error

		wantFlushed    bool
		wantHijacked   bool
		wantStatusCode int
	}{
		{
			name: "flush",
			call: (*http.ResponseController).Flush,

			wantFlushed:    true,
			wantHijacked:   false,
			wantStatusCode: 0,
		},
		{
			name: "hijack",
			call: func(rc *http.ResponseController) error {
				_, _, err := rc.Hijack()
				return err
			},

			wantFlushed:    false,
			wantHijacked:   true,
			wantStatusCode: http.StatusSwitchingProtocols,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			w := &controllerWriter{ResponseWriter: httptest.NewRecorder()}
			rec := &statusRecorder{ResponseWriter: w}

			// Run test
			err := tc.call(http.NewResponseController(rec))

			// Assert
			require.NoError(t, err, "the response controller reaches the wrapped writer")
			require.Equal(t, tc.wantFlushed, w.flushed, "the wrapped writer is flushed")
			require.Equal(t, tc.wantHijacked, w.hijacked, "the wrapped writer is hijacked")
			require.Equal(t, tc.wantStatusCode, rec.statusCode, "a hijack records the code 101")
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

func observations(t *testing.T, method string, route string) uint64 {
	t.Helper()

	reg := prometheus.NewRegistry()
	reg.MustRegister(httpRequestDuration)

	families, err := reg.Gather()
	require.NoError(t, err, "gather the duration histogram")

	var count uint64
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			if labels["method"] == method && labels["route"] == route {
				count += metric.GetHistogram().GetSampleCount()
			}
		}
	}

	return count
}

func noop(http.ResponseWriter, *http.Request) {}

func writeBody(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

func writeStatus(statusCodes ...int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, statusCode := range statusCodes {
			w.WriteHeader(statusCode)
		}
	}
}

func panicWith(value any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		panic(value)
	}
}

func writeBodyThenPanic(body string, value any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))

		panic(value)
	}
}

func writeStatusThenPanic(statusCode int, value any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)

		panic(value)
	}
}

// hijack behaves like httputil.ReverseProxy. It writes the code 101 to the
// hijacked connection and not with WriteHeader.
func hijack(w http.ResponseWriter, r *http.Request) {
	_, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
	}
}

func hijackThenPanic(value any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _, _ = http.NewResponseController(w).Hijack()

		panic(value)
	}
}
