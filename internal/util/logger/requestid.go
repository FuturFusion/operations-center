package logger

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type requestIDKey struct{}

// RequestIDMiddleware assigns an ID to the request, which is reported in every
// log record emitted while serving it. The ID is also reported to the client,
// both in the response header and in the metadata of error responses, which
// allows to correlate an error reported to the user with the log.
func RequestIDMiddleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestUUID, _ := uuid.NewRandom()
		requestID := requestUUID.String()

		ctx = context.WithValue(ctx, requestIDKey{}, requestID)
		ctx = ContextWithAttr(ctx, slog.String("request_id", requestID))

		w.Header().Set(api.RequestIDHeader, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(handlerFunc)
}

// RequestIDFromContext returns the ID of the request the given context belongs
// to or an empty string, if the context does not belong to a request.
func RequestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDKey{}).(string)
	if !ok {
		return ""
	}

	return requestID
}
