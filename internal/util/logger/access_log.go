package logger

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

var componentAccessLog = RegisterComponent("access_log")

func AccessLogMiddleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		rw := &responseRecorder{
			w: w,
		}

		defer func() {
			slog.InfoContext(
				ContextWithComponent(r.Context(), componentAccessLog),
				"access log",
				slog.String("ip", r.RemoteAddr),
				slog.String("method", r.Method),
				slog.String("request_uri", r.RequestURI),
				slog.Int("status_code", rw.statusCode),
				slog.Int("response_size", rw.responseSize),
			)
		}()

		next.ServeHTTP(rw, r)
	}

	return http.HandlerFunc(handlerFunc)
}

type responseRecorder struct {
	statusCode   int
	responseSize int
	w            http.ResponseWriter
}

// Hijack implements [http.Hijacker], the cowsql gateway hijacks the database connections.
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("Response writer does not support hijacking: %T", r.w)
	}

	return h.Hijack()
}

func (r *responseRecorder) Header() http.Header {
	return r.w.Header()
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	count, err := r.w.Write(b)
	r.responseSize += count
	return count, err
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.w.WriteHeader(statusCode)
}
