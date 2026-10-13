package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestHTTPMiddleware(t *testing.T) {
	serve := func(method string, target string) func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
		return func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
			t.Helper()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))

			return rec
		}
	}

	servePanic := func(target string) func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
		return func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
			t.Helper()

			rec := httptest.NewRecorder()
			require.PanicsWithValue(t, http.ErrAbortHandler, func() {
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
			}, "the middleware does not recover the panic")

			return rec
		}
	}

	tests := []struct {
		name              string
		enabledComponents []string
		request           func(t *testing.T, handler http.Handler) *httptest.ResponseRecorder

		wantStatusCode   int
		wantBody         string
		wantMethod       string
		wantRoute        string
		wantCode         string
		wantSeries       int
		wantErrorSeries  int
		wantRequests     float64
		wantErrors       float64
		wantAborted      float64
		wantObservations uint64
	}{
		{
			name:              "success - matched route",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/items/42"),

			wantStatusCode:   http.StatusOK,
			wantBody:         "item",
			wantMethod:       http.MethodGet,
			wantRoute:        "/items/{id}",
			wantCode:         "200",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - response without explicit status code",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/empty"),

			wantStatusCode:   http.StatusOK,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/empty",
			wantCode:         "200",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - body without explicit status code",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/write"),

			wantStatusCode:   http.StatusOK,
			wantBody:         "body",
			wantMethod:       http.MethodGet,
			wantRoute:        "/write",
			wantCode:         "200",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - pattern without method",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/proxy/one/path"),

			wantStatusCode:   http.StatusOK,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/proxy/{name}/",
			wantCode:         "200",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - redirect to the clean path",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/items//42"),

			wantStatusCode:   http.StatusTemporaryRedirect,
			wantBody:         "<a href=\"/items/42\">Temporary Redirect</a>.\n\n",
			wantMethod:       http.MethodGet,
			wantRoute:        "/items/{id}",
			wantCode:         "307",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - unmatched route",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/does/not/exist"),

			wantStatusCode:   http.StatusNotFound,
			wantBody:         "404 page not found\n",
			wantMethod:       http.MethodGet,
			wantRoute:        routeUnmatched,
			wantCode:         "404",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - unknown method",
			enabledComponents: []string{"api"},
			request:           serve("BREW", "/does/not/exist"),

			wantStatusCode:   http.StatusNotFound,
			wantBody:         "404 page not found\n",
			wantMethod:       methodOther,
			wantRoute:        routeUnmatched,
			wantCode:         "404",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - method not allowed",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodPost, "/items/42"),

			wantStatusCode:   http.StatusMethodNotAllowed,
			wantBody:         "Method Not Allowed\n",
			wantMethod:       http.MethodPost,
			wantRoute:        routeUnmatched,
			wantCode:         "405",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - informational status code before the final one",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/early-hints"),

			// httptest.ResponseRecorder keeps the first status code.
			wantStatusCode:   http.StatusEarlyHints,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/early-hints",
			wantCode:         "200",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - response aborts after the body started",
			enabledComponents: []string{"api"},
			request:           servePanic("/write-then-panic"),

			wantStatusCode:   http.StatusOK,
			wantBody:         "body",
			wantMethod:       http.MethodGet,
			wantRoute:        "/write-then-panic",
			wantCode:         "200",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      1,
			wantObservations: 1,
		},
		{
			name:              "success - response aborts after the status code",
			enabledComponents: []string{"api"},
			request:           servePanic("/created-then-panic"),

			wantStatusCode:   http.StatusCreated,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/created-then-panic",
			wantCode:         "201",
			wantSeries:       1,
			wantErrorSeries:  0,
			wantRequests:     1,
			wantErrors:       0,
			wantAborted:      1,
			wantObservations: 1,
		},
		{
			name:              "error - status 500",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/fail"),

			wantStatusCode:   http.StatusInternalServerError,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/fail",
			wantCode:         "500",
			wantSeries:       1,
			wantErrorSeries:  1,
			wantRequests:     1,
			wantErrors:       1,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "error - status 503",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/unavailable"),

			wantStatusCode:   http.StatusServiceUnavailable,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/unavailable",
			wantCode:         "503",
			wantSeries:       1,
			wantErrorSeries:  1,
			wantRequests:     1,
			wantErrors:       1,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "error - informational status code before status 500",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/early-hints-then-fail"),

			// httptest.ResponseRecorder keeps the first status code.
			wantStatusCode:   http.StatusEarlyHints,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/early-hints-then-fail",
			wantCode:         "500",
			wantSeries:       1,
			wantErrorSeries:  1,
			wantRequests:     1,
			wantErrors:       1,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "error - first status code wins",
			enabledComponents: []string{"api"},
			request:           serve(http.MethodGet, "/fail-twice"),

			wantStatusCode:   http.StatusInternalServerError,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/fail-twice",
			wantCode:         "500",
			wantSeries:       1,
			wantErrorSeries:  1,
			wantRequests:     1,
			wantErrors:       1,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "error - handler panics before the response",
			enabledComponents: []string{"api"},
			request:           servePanic("/panic"),

			wantStatusCode:   http.StatusOK,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/panic",
			wantCode:         "500",
			wantSeries:       1,
			wantErrorSeries:  1,
			wantRequests:     1,
			wantErrors:       1,
			wantAborted:      0,
			wantObservations: 1,
		},
		{
			name:              "success - component disabled",
			enabledComponents: []string{"runtime"},
			request:           serve(http.MethodGet, "/fail"),

			wantStatusCode:   http.StatusInternalServerError,
			wantBody:         "",
			wantMethod:       http.MethodGet,
			wantRoute:        "/fail",
			wantCode:         "500",
			wantSeries:       0,
			wantErrorSeries:  0,
			wantRequests:     0,
			wantErrors:       0,
			wantAborted:      0,
			wantObservations: 0,
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
			mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("item"))
			})

			mux.HandleFunc("GET /empty", func(w http.ResponseWriter, r *http.Request) {})
			mux.HandleFunc("GET /write", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("body"))
			})

			mux.HandleFunc("/proxy/{name}/", func(w http.ResponseWriter, r *http.Request) {})
			mux.HandleFunc("GET /early-hints", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusEarlyHints)
				w.WriteHeader(http.StatusOK)
			})

			mux.HandleFunc("GET /fail", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})

			mux.HandleFunc("GET /unavailable", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			})

			mux.HandleFunc("GET /early-hints-then-fail", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusEarlyHints)
				w.WriteHeader(http.StatusInternalServerError)
			})

			mux.HandleFunc("GET /fail-twice", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.WriteHeader(http.StatusOK)
			})

			mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
				panic(http.ErrAbortHandler)
			})

			mux.HandleFunc("GET /write-then-panic", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("body"))

				panic(http.ErrAbortHandler)
			})

			mux.HandleFunc("GET /created-then-panic", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)

				panic(http.ErrAbortHandler)
			})

			// Run test
			rec := tc.request(t, HTTPMiddleware(mux))

			// Assert
			require.Equal(t, tc.wantStatusCode, rec.Code, "status code of the response")
			require.Equal(t, tc.wantBody, rec.Body.String(), "body of the response")
			require.Equal(t, tc.wantSeries, testutil.CollectAndCount(httpRequestsTotal), "number of request counter series")
			require.Equal(t, tc.wantErrorSeries, testutil.CollectAndCount(httpRequestErrorsTotal), "number of error counter series")
			require.Equal(t, tc.wantSeries, testutil.CollectAndCount(httpRequestDuration), "number of duration histogram series")
			require.InDelta(t, tc.wantRequests, testutil.ToFloat64(httpRequestsTotal.WithLabelValues(tc.wantMethod, tc.wantRoute, tc.wantCode)), 0, "request counter")
			require.InDelta(t, tc.wantErrors, testutil.ToFloat64(httpRequestErrorsTotal.WithLabelValues(tc.wantMethod, tc.wantRoute)), 0, "error counter")
			require.InDelta(t, tc.wantAborted, testutil.ToFloat64(httpRequestsAbortedTotal.WithLabelValues(tc.wantMethod, tc.wantRoute)), 0, "aborted counter")
			require.Equal(t, tc.wantObservations, observations(t, tc.wantMethod, tc.wantRoute), "number of duration observations")
		})
	}
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

func TestStatusRecorder_Unwrap(t *testing.T) {
	// Setup
	rec := httptest.NewRecorder()

	// Run test
	err := http.NewResponseController(&statusRecorder{ResponseWriter: rec}).Flush()

	// Assert
	require.NoError(t, err, "the response controller reaches the wrapped writer")
	require.True(t, rec.Flushed, "the wrapped writer is flushed")
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
