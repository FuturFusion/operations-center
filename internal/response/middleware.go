package response

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/FuturFusion/operations-center/internal/logger"
)

func With(handler HandlerFunc, middlewares ...func(next HandlerFunc) HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := slog.With(slog.String("method", r.Method), slog.String("url", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		next := handler
		for _, middleware := range slices.Backward(middlewares) {
			next = middleware(next)
		}

		resp := next(r)

		err := resp.Render(w)
		if err != nil {
			writeErr := SmartError(err).Render(w)
			if writeErr != nil {
				log.Error("Failed writing error for HTTP response", logger.Err(err), slog.Any("write_err", writeErr))
				return
			}
		}
	}
}
