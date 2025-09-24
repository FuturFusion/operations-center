package response

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/FuturFusion/operations-center/internal/logger"
)

func With(handler HandlerFunc, middlewares ...func(next HandlerFunc) HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := slog.With(slog.String("method", r.Method), slog.String("request_uri", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		next := handler
		for _, middleware := range slices.Backward(middlewares) {
			next = middleware(next)
		}

		resp := next(r)
		switch {
		case resp.Code() >= 400 && resp.Code() < 500:
			log.WarnContext(r.Context(), "Client error response", slog.Int("status_code", resp.Code()), slog.String("response", resp.String()))
		case resp.Code() >= 500 && resp.Code() < 600:
			log.ErrorContext(r.Context(), "Server error response", slog.Int("status_code", resp.Code()), slog.String("response", resp.String()))
		default:
			// Response content is omitted, since it might be huge.
			log.DebugContext(r.Context(), "Response", slog.Int("status_code", resp.Code()))
		}

		err := resp.Render(w)
		if err != nil {
			writeErr := SmartError(err).Render(w)
			if writeErr != nil {
				log.ErrorContext(r.Context(), "Failed writing error for HTTP response", logger.Err(err), slog.Any("write_err", writeErr))
				return
			}

			log.ErrorContext(r.Context(), "Render error")
		}
	}
}
