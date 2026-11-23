package metrics

import (
	"bufio"
	"net"
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
		Help:      "Number of HTTP requests.",
	}, []string{"method", "route", "code"})

	httpRequestErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "http",
		Name:      "request_errors_total",
		Help:      "Number of HTTP requests with a server error (status code 500 or higher).",
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
		Help:      "Duration of the HTTP requests in seconds.",
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

// HTTPMiddleware counts the requests, server errors and aborted responses of
// next and records their duration.
//
// It reads the route from the pattern that http.ServeMux sets on the request.
// http.ServeMux sets the pattern on the request that it receives. Do not place
// a middleware between them that copies the request, for example with
// r.WithContext. This middleware cannot see the pattern on a copy.
//
// A hijacked connection records the code 101. Its duration ends with the
// hijack. It does not include the session after the hijack.
//
// A response that aborts after it starts keeps its status code. A client
// disconnect causes the same abort. So the abort itself is not a server error.
// A separate counter records it. A panic with a value other than
// http.ErrAbortHandler is a bug in the handler. It counts as a server error.
// The middleware panics again with the same value.
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
			recovered := recover()

			end := time.Now()
			if !rec.hijackedAt.IsZero() {
				end = rec.hijackedAt
			}

			duration := end.Sub(start)

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
				// The handler did not return, for example because it panicked.
				// No response started.
				statusCode = http.StatusInternalServerError
			}

			httpRequestsTotal.WithLabelValues(method, route, strconv.Itoa(statusCode)).Inc()
			httpRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())

			crashed := recovered != nil && recovered != http.ErrAbortHandler
			if statusCode >= http.StatusInternalServerError || crashed {
				httpRequestErrorsTotal.WithLabelValues(method, route).Inc()
			}

			if recovered != nil {
				panic(recovered)
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
	// http.ServeMux accepts spaces and tabs after the method.
	route := pattern
	i := strings.IndexAny(pattern, " \t")
	if i >= 0 {
		route = strings.TrimLeft(pattern[i:], " \t")
	}

	if route == "" {
		return routeUnmatched
	}

	return route
}

type statusRecorder struct {
	http.ResponseWriter

	statusCode int
	hijackedAt time.Time
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	// An informational status code is not the final status code. The handler
	// sends another one after it. 101 Switching Protocols is final.
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

// Hijack records the hijack of the connection. http.ResponseController
// prefers Hijack over Unwrap. A handler like httputil.ReverseProxy writes the
// code 101 to the hijacked connection and never calls WriteHeader.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(r.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}

	if r.statusCode == 0 {
		r.statusCode = http.StatusSwitchingProtocols
	}

	r.hijackedAt = time.Now()

	return conn, rw, nil
}

// Unwrap gives http.ResponseController access to the wrapped writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
