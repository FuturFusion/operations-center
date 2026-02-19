package logger

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

func RequestIDMiddleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID, _ := uuid.NewRandom()

		ctx = ContextWithAttr(ctx, slog.String("request_id", requestID.String()))
		next.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(handlerFunc)
}
