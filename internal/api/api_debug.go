package api

import (
	"net/http"
	"net/http/pprof"
	runtimepprof "runtime/pprof"
	"strconv"
	"sync"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

const pprofMaxSeconds = 300

type debugHandler struct {
	// captureMu allows a single CPU profile or trace capture at a time.
	captureMu sync.Mutex
}

// registerDebugHandler registers the pprof handlers explicitly, so they are
// never exposed through http.DefaultServeMux.
func registerDebugHandler(router Router, authorizer *authz.Authorizer) {
	handler := &debugHandler{}

	router.HandleFunc("GET /pprof/{$}", response.With(pprofResponse(pprof.Index), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
	router.HandleFunc("GET /pprof/cmdline", response.With(pprofResponse(pprof.Cmdline), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
	router.HandleFunc("GET /pprof/symbol", response.With(pprofResponse(pprof.Symbol), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
	router.HandleFunc("POST /pprof/symbol", response.With(pprofResponse(pprof.Symbol), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
	router.HandleFunc("GET /pprof/profile", response.With(handler.pprofCapture(pprof.Profile), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
	router.HandleFunc("GET /pprof/trace", response.With(handler.pprofCapture(pprof.Trace), assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
	router.HandleFunc("GET /pprof/{name}", response.With(handler.pprofProfileGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDebug), assertPprofEnabled))
}

func assertPprofEnabled(next response.HandlerFunc) response.HandlerFunc {
	return func(r *http.Request) response.Response {
		if !config.GetSettings().PprofEnabled {
			return response.SmartError(domain.NewErrorf(domain.ErrOperationNotPermitted, "", "The pprof debug endpoints are disabled").
				WithHintf(`Enable them with the "pprof_enabled" system setting.`))
		}

		return next(r)
	}
}

func pprofResponse(handler http.HandlerFunc) response.HandlerFunc {
	return func(r *http.Request) response.Response {
		return response.ManualResponse(func(w http.ResponseWriter) error {
			handler(w, r)

			return nil
		})
	}
}

// pprofCapture guards the handlers, which block for the requested duration
// while capturing.
func (d *debugHandler) pprofCapture(handler http.HandlerFunc) response.HandlerFunc {
	return func(r *http.Request) response.Response {
		err := validatePprofSeconds(r)
		if err != nil {
			return response.SmartError(err)
		}

		if !d.captureMu.TryLock() {
			return response.SmartError(domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Another pprof capture is already running").
				WithHintf("Retry once the running capture has finished."))
		}

		return response.ManualResponse(func(w http.ResponseWriter) error {
			defer d.captureMu.Unlock()

			handler(w, r)

			return nil
		})
	}
}

func (d *debugHandler) pprofProfileGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	if runtimepprof.Lookup(name) == nil {
		return response.SmartError(domain.NewErrorf(domain.ErrNotFound, "", "Unknown pprof profile %q", name).
			WithDetail("name", name))
	}

	err := validatePprofSeconds(r)
	if err != nil {
		return response.SmartError(err)
	}

	return pprofResponse(pprof.Handler(name).ServeHTTP)(r)
}

func validatePprofSeconds(r *http.Request) error {
	value := r.FormValue("seconds")
	if value == "" {
		return nil
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 || seconds > pprofMaxSeconds {
		return domain.NewValidationErrf("Invalid value %q for query parameter %q, expected a number of seconds between 1 and %d", value, "seconds", pprofMaxSeconds)
	}

	return nil
}
