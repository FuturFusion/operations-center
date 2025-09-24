package logger

import (
	"log/slog"
	"net/http"
)

func AccessLogMiddleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		rw := &responseRecorder{
			w: w,
		}

		defer func() {
			slog.InfoContext(r.Context(), "access log",
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
