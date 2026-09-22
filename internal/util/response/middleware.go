package response

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

func With(handler HandlerFunc, middlewares ...func(next HandlerFunc) HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := slog.With(slog.String("method", r.Method), slog.String("request_uri", r.URL.RequestURI()), slog.String("ip", r.RemoteAddr))

		next := handler
		for _, middleware := range slices.Backward(middlewares) {
			next = middleware(next)
		}

		resp := next(r)

		logResponse(r.Context(), log, resp)

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

// logResponse reports the outcome of the request. It is the single place, where
// a failed request is logged, the layers below report their errors to the
// caller instead of logging them.
func logResponse(ctx context.Context, log *slog.Logger, resp Response) {
	statusCode := resp.Code()

	switch {
	case statusCode == -1:
		log.DebugContext(ctx, "Manual response, response code unknown")

	case statusCode >= 400 && statusCode < 600:
		attrs := []slog.Attr{
			slog.Int("status_code", statusCode),
			slog.String("response", resp.String()),
		}

		errResp, ok := resp.(*errorResponse)
		if ok {
			attrs = append(attrs, slog.String("reason", string(errResp.reason)))

			if len(errResp.details) > 0 {
				attrs = append(attrs, slog.Any("details", errResp.details))
			}

			if errResp.err != nil {
				attrs = append(attrs, logger.Err(errResp.err))
			}
		}

		level := slog.LevelWarn
		if statusCode >= 500 {
			level = slog.LevelError
		}

		log.LogAttrs(ctx, level, "Request failed", attrs...)

	default:
		// Response content is omitted, since it might be huge.
		log.DebugContext(ctx, "Response", slog.Int("status_code", statusCode))
	}
}
