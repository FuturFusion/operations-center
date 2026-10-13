package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/task"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

var (
	componentAPI    = logger.RegisterComponent("api")
	componentDaemon = logger.RegisterComponent("daemon")

	componentTaskCertificatesValidityCheck = logger.RegisterComponent("daemon.task.certificates_validity_check")
	componentTaskClusterUpdate             = logger.RegisterComponent("daemon.task.cluster_update")
	componentTaskPollEvacuatingServers     = logger.RegisterComponent("daemon.task.poll_evacuating_servers")
	componentTaskPollPendingServers        = logger.RegisterComponent("daemon.task.poll_pending_servers")
	componentTaskPollReadyServers          = logger.RegisterComponent("daemon.task.poll_ready_servers")
	componentTaskPollRebootingServers      = logger.RegisterComponent("daemon.task.poll_rebooting_servers")
	componentTaskPollRestoringServers      = logger.RegisterComponent("daemon.task.poll_restoring_servers")
	componentTaskPollUnresponsiveServers   = logger.RegisterComponent("daemon.task.poll_unresponsive_servers")
	componentTaskPollUpdatingAppServers    = logger.RegisterComponent("daemon.task.poll_updating_application_servers")
	componentTaskPollUpdatingServers       = logger.RegisterComponent("daemon.task.poll_updating_servers")
	componentTaskPruneSeedImageCache       = logger.RegisterComponent("daemon.task.prune_seed_image_cache")
	componentTaskRefreshBMCData            = logger.RegisterComponent("daemon.task.refresh_bmc_data")
	componentTaskRefreshImageSources       = logger.RegisterComponent("daemon.task.refresh_image_sources")
	componentTaskRefreshInventory          = logger.RegisterComponent("daemon.task.refresh_inventory")
	componentTaskRefreshUpdates            = logger.RegisterComponent("daemon.task.refresh_updates")
	componentTaskRenewACMECertificate      = logger.RegisterComponent("daemon.task.renew_acme_certificate")
	componentTaskServerDeployment          = logger.RegisterComponent("daemon.task.server_deployment")

	componentAuthnTLS        = logger.RegisterComponent("security.authn.tls")
	componentAuthnUnixSocket = logger.RegisterComponent("security.authn.unixsocket")

	componentAuthzChain      = logger.RegisterComponent("security.authz.chain")
	componentAuthzOIDC       = logger.RegisterComponent("security.authz.oidc")
	componentAuthzTLS        = logger.RegisterComponent("security.authz.tls")
	componentAuthzUnixSocket = logger.RegisterComponent("security.authz.unixsocket")
)

// componentMiddleware scopes the request context to the api component, so log
// records emitted while serving a request are attributed to it. A service
// reached by the request overrides the component with its own.
func componentMiddleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(logger.ContextWithComponent(r.Context(), componentAPI)))
	}

	return http.HandlerFunc(handlerFunc)
}

// componentTask scopes a background task to the given component, so everything
// it logs is attributed to it, down to the services it calls.
func componentTask(component logger.Component, f task.Func) task.Func {
	return func(ctx context.Context) {
		f(logger.ContextWithComponent(ctx, component))
	}
}

// registerComponentsValidation registers the checks for unknown components.
// The validate signal is not emitted while the configuration loads. A removed
// component therefore does not stop the daemon from starting.
var registerComponentsValidation = sync.OnceFunc(func() {
	lifecycle.SettingsValidateSignal.AddListenerWithErr(validateComponentLevels)
	lifecycle.SettingsValidateSignal.AddListenerWithErr(validateMetricsComponents)
})

// validateComponentLevels rejects log levels configured for unknown components.
func validateComponentLevels(_ context.Context, settings apisystem.Settings) error {
	err := logger.ValidateComponentLevelsKnown(settings.LogLevels)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.log_levels" property is invalid: %v`, err)
	}

	return nil
}

// validateMetricsComponents rejects metrics enabled for unknown components.
func validateMetricsComponents(_ context.Context, settings apisystem.Settings) error {
	err := metrics.ValidateComponentsKnown(settings.MetricsComponents)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.metrics_components" property is invalid: %v`, err)
	}

	return nil
}
