package api

import (
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// shutdownGate refuses changes once the daemon shuts down. Reading requests, the internal API and the IncusOS API are still handled.
func shutdownGate(shuttingDown *atomic.Bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shuttingDown.Load() && !allowedDuringShutdown(r) {
			err := response.SmartError(domain.NewErrorf(domain.ErrUnavailable, "", "Operations Center is shutting down")).Render(w)
			if err != nil {
				slog.WarnContext(r.Context(), "Render error response failed", logger.Err(err))
			}

			return
		}

		next.ServeHTTP(w, r)
	})
}

// allowedDuringShutdown reports whether r is handled while the daemon shuts down.
func allowedDuringShutdown(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}

	for _, prefix := range []string{"/internal", "/os"} {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			return true
		}
	}

	return false
}
