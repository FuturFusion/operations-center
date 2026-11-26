package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

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

	// Use string literals so cmd/generate-components can discover both component lists.
	componentTaskCertificatesValidityCheck = newTaskComponent(
		logger.RegisterComponent("daemon.task.certificates_validity_check"),
		metrics.RegisterComponent("daemon.task.certificates_validity_check"),
	)
	componentTaskClusterUpdate = newTaskComponent(
		logger.RegisterComponent("daemon.task.cluster_update"),
		metrics.RegisterComponent("daemon.task.cluster_update"),
	)
	componentTaskPollEvacuatingServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_evacuating_servers"),
		metrics.RegisterComponent("daemon.task.poll_evacuating_servers"),
	)
	componentTaskPollPendingServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_pending_servers"),
		metrics.RegisterComponent("daemon.task.poll_pending_servers"),
	)
	componentTaskPollReadyServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_ready_servers"),
		metrics.RegisterComponent("daemon.task.poll_ready_servers"),
	)
	componentTaskPollRebootingServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_rebooting_servers"),
		metrics.RegisterComponent("daemon.task.poll_rebooting_servers"),
	)
	componentTaskPollRestoringServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_restoring_servers"),
		metrics.RegisterComponent("daemon.task.poll_restoring_servers"),
	)
	componentTaskPollUnresponsiveServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_unresponsive_servers"),
		metrics.RegisterComponent("daemon.task.poll_unresponsive_servers"),
	)
	componentTaskPollUpdatingAppServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_updating_application_servers"),
		metrics.RegisterComponent("daemon.task.poll_updating_application_servers"),
	)
	componentTaskPollUpdatingServers = newTaskComponent(
		logger.RegisterComponent("daemon.task.poll_updating_servers"),
		metrics.RegisterComponent("daemon.task.poll_updating_servers"),
	)
	componentTaskPruneSecureBootMedia = newTaskComponent(
		logger.RegisterComponent("daemon.task.prune_secure_boot_media"),
		metrics.RegisterComponent("daemon.task.prune_secure_boot_media"),
	)
	componentTaskPruneSeedImageCache = newTaskComponent(
		logger.RegisterComponent("daemon.task.prune_seed_image_cache"),
		metrics.RegisterComponent("daemon.task.prune_seed_image_cache"),
	)
	componentTaskRefreshBMCData = newTaskComponent(
		logger.RegisterComponent("daemon.task.refresh_bmc_data"),
		metrics.RegisterComponent("daemon.task.refresh_bmc_data"),
	)
	componentTaskRefreshImageSources = newTaskComponent(
		logger.RegisterComponent("daemon.task.refresh_image_sources"),
		metrics.RegisterComponent("daemon.task.refresh_image_sources"),
	)
	componentTaskRefreshInventory = newTaskComponent(
		logger.RegisterComponent("daemon.task.refresh_inventory"),
		metrics.RegisterComponent("daemon.task.refresh_inventory"),
	)
	componentTaskRefreshUpdates = newTaskComponent(
		logger.RegisterComponent("daemon.task.refresh_updates"),
		metrics.RegisterComponent("daemon.task.refresh_updates"),
	)
	componentTaskRenewACMECertificate = newTaskComponent(
		logger.RegisterComponent("daemon.task.renew_acme_certificate"),
		metrics.RegisterComponent("daemon.task.renew_acme_certificate"),
	)
	componentTaskServerDeployment = newTaskComponent(
		logger.RegisterComponent("daemon.task.server_deployment"),
		metrics.RegisterComponent("daemon.task.server_deployment"),
	)

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

type taskComponent struct {
	log     logger.Component
	metrics *metrics.Task
}

func newTaskComponent(log logger.Component, metricsComponent *metrics.Component) taskComponent {
	if string(log) != metricsComponent.Name() {
		panic(fmt.Sprintf("api: log component %q and metrics component %q of a task differ", log, metricsComponent.Name()))
	}

	return taskComponent{
		log:     log,
		metrics: metrics.NewTask(metricsComponent, strings.TrimPrefix(metricsComponent.Name(), "daemon.task.")),
	}
}

func componentTask(c taskComponent, interval, stopDeadline time.Duration, f func(context.Context) error, opts ...task.EveryOption) task.TaskFunc {
	// f logs failures; its return value is used only to record metrics.
	run := func(ctx context.Context) {
		start := time.Now()
		err := f(logger.ContextWithComponent(ctx, c.log))
		c.metrics.Observe(ctx, start, err)
	}

	startTask := task.New(run, task.Every(interval, opts...), stopDeadline)

	return func(ctx context.Context) func(context.Context) error {
		c.metrics.Started(interval)
		stopTask := startTask(ctx)

		return func(ctx context.Context) error {
			// Hide leadership gauges before waiting for the canceled run to finish.
			c.metrics.Stopped()

			return stopTask(ctx)
		}
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
func validateComponentLevels(_ context.Context, settings apisystem.SettingsLocal) error {
	err := logger.ValidateComponentLevelsKnown(settings.LogLevels)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.log_levels" property is invalid: %v`, err)
	}

	return nil
}

// validateMetricsComponents rejects metrics enabled for unknown components.
func validateMetricsComponents(_ context.Context, settings apisystem.SettingsLocal) error {
	err := metrics.ValidateComponentsKnown(settings.MetricsComponents)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, "settings.metrics_components" property is invalid: %v`, err)
	}

	return nil
}
