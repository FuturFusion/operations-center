package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var componentAPI = RegisterComponent("api")

const (
	routeUnmatched = "unmatched"
	methodOther    = "other"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Number of HTTP requests served by Operations Center.",
	}, []string{"method", "route", "code"})

	httpRequestErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "http",
		Name:      "request_errors_total",
		Help:      "Number of HTTP requests answered with a server error (status code 500 or higher).",
	}, []string{"method", "route"})

	httpRequestsAbortedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "http",
		Name:      "requests_aborted_total",
		Help:      "Number of HTTP requests with a response that aborted after it started.",
	}, []string{"method", "route"})

	httpRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "Duration in seconds of HTTP requests served by Operations Center.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})
)

func init() {
	registry.MustRegister(
		GatedCollector(componentAPI, httpRequestsTotal),
		GatedCollector(componentAPI, httpRequestErrorsTotal),
		GatedCollector(componentAPI, httpRequestsAbortedTotal),
		GatedCollector(componentAPI, httpRequestDuration),
	)
}

// HTTPMiddleware records the rate, the errors and the duration of the requests
// served by next.
//
// It reads the route from the pattern that http.ServeMux sets on the request.
// Wrap the http.ServeMux directly. Do not place a middleware between them. A
// copy of the request does not receive the pattern.
//
// A response that aborts after it started keeps its status code. The same
// abort happens when the client disconnects, so it is not a server error. A
// separate counter records it.
func HTTPMiddleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		if !componentAPI.Enabled() {
			next.ServeHTTP(w, r)
			return
		}

		rec := &statusRecorder{ResponseWriter: w}
		start := time.Now()
		completed := false

		defer func() {
			duration := time.Since(start)

			method := methodLabel(r.Method)
			route := routeLabel(r.Pattern)

			statusCode := rec.statusCode
			started := statusCode != 0

			switch {
			case started && !completed:
				httpRequestsAbortedTotal.WithLabelValues(method, route).Inc()

			case !started && completed:
				statusCode = http.StatusOK

			case !started:
				// The handler panicked before it started the response.
				statusCode = http.StatusInternalServerError
			}

			httpRequestsTotal.WithLabelValues(method, route, strconv.Itoa(statusCode)).Inc()
			httpRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())

			if statusCode >= http.StatusInternalServerError {
				httpRequestErrorsTotal.WithLabelValues(method, route).Inc()
			}
		}()

		next.ServeHTTP(rec, r)

		completed = true
	}

	return http.HandlerFunc(handlerFunc)
}

func methodLabel(method string) string {
	// The method is client input. Map unknown methods to one value to limit
	// the number of series.
	switch method {
	case http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace:
		return method

	default:
		return methodOther
	}
}

func routeLabel(pattern string) string {
	// The request path is client input. Use only the matched pattern. Drop the
	// method from the pattern. The method has its own label.
	_, route, found := strings.Cut(pattern, " ")
	if !found {
		route = pattern
	}

	route = strings.TrimLeft(route, " \t")
	if route == "" {
		return routeUnmatched
	}

	return route
}

type statusRecorder struct {
	http.ResponseWriter

	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	// An informational status code is not the final status code. The handler
	// sends another one after it.
	final := statusCode >= http.StatusOK || statusCode == http.StatusSwitchingProtocols
	if r.statusCode == 0 && final {
		r.statusCode = statusCode
	}

	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}

	return r.ResponseWriter.Write(p)
}

// Unwrap gives http.ResponseController access to the wrapped writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
