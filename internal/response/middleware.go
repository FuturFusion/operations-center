package response

import (
	"log/slog"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/logger"
)

func With(handler func(r *http.Request) Response) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := slog.With(slog.String("method", r.Method), slog.String("url", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		resp := handler(r)

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
