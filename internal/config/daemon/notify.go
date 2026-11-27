package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
)

var componentConfig = logger.RegisterComponent("config")

// notify informs the subsystems about a configuration that has already been
// persisted and committed. It can not fail the update anymore, so everything
// that may reject a configuration belongs in validate or validateDelegated.
// Failures here are logged for a change of this member and reported for a
// change reloaded from another member, which reverts it.
//
// The payloads are taken from newCfg, the committed configuration, so listeners
// see the same values as GetNetwork and friends, defaults included.
//
// The section the update was addressed to is always notified, even if the values
// did not change, so re-submitting a configuration applies it again. That is how
// an operator recovers a subsystem which failed to pick up an earlier update.
// Every other section is only notified if it actually changed, so an update of
// one section does not disturb the others.
func notifyLocal(ctx context.Context, updated section, oldCfg, newCfg localConfig) error {
	// The component is only applied to the records emitted here. The context
	// passed to the listeners keeps the component of the caller, so they are
	// not attributed to the config package.
	logCtx := logger.ContextWithComponent(ctx, componentConfig)

	if updated == sectionNetwork || isNetworkChanged(oldCfg, newCfg) {
		lifecycle.NetworkUpdateSignal.Emit(ctx, newCfg.Network)
	}

	if isLogLevelChanged(oldCfg, newCfg) {
		err := logger.SetLogLevel(logger.ParseLevel(newCfg.Settings.LogLevel))
		if err != nil {
			slog.ErrorContext(logCtx, "Failed to apply log level from updated config", logger.Err(err))
		}
	}

	if isLogLevelsChanged(oldCfg, newCfg) {
		err := logger.SetComponentLevels(logger.ParseComponentLevels(newCfg.Settings.LogLevels))
		if err != nil {
			slog.ErrorContext(logCtx, "Failed to apply per component log levels from updated config", logger.Err(err))
		}
	}

	if isPprofEnabledChanged(oldCfg, newCfg) {
		slog.WarnContext(logCtx, "Updated pprof debug endpoints setting", slog.Bool("enabled", newCfg.Settings.PprofEnabled))
	}

	if isMetricsChanged(oldCfg, newCfg) {
		slog.WarnContext(logCtx, "Updated metrics settings", slog.Bool("enabled", newCfg.Settings.MetricsEnabled), slog.Any("components", newCfg.Settings.MetricsComponents))

		metrics.Apply(newCfg.Settings.MetricsEnabled, newCfg.Settings.MetricsComponents)
	}

	return nil
}

// notifyShared applies a shared configuration and returns the errors of the listeners failing to.
// Every listener is run, a failed one does not keep the others from applying the configuration.
func notifyShared(ctx context.Context, updated section, oldCfg, newCfg SharedConfig) error {
	var errs []error

	if updated == sectionNetwork || isSharedNetworkChanged(oldCfg, newCfg) {
		errs = append(errs, applied("network", lifecycle.SharedNetworkUpdateSignal.TryEmit(ctx, newCfg.Network)))
	}

	if isSecurityAuthChanged(oldCfg, newCfg) {
		errs = append(errs, applied("security", lifecycle.SecurityUpdateSignal.TryEmit(ctx, newCfg.Security)))
	}

	if isTrustedHTTPSProxiesChanged(oldCfg, newCfg) {
		errs = append(errs, applied("trusted HTTPS proxies", lifecycle.SecurityTrustedHTTPSProxiesUpdateSignal.TryEmit(ctx, newCfg.Security.TrustedHTTPSProxies)))
	}

	// The certificate already matches the stored ACME config.
	if updated != sectionLoaded && isACMEChanged(oldCfg, newCfg) {
		errs = append(errs, applied("ACME", lifecycle.SecurityACMEUpdateSignal.TryEmit(ctx, newCfg.Security.ACME)))
	}

	if updated == sectionSettings || isSharedSettingsChanged(oldCfg, newCfg) {
		errs = append(errs, applied("settings", lifecycle.SharedSettingsUpdateSignal.TryEmit(ctx, newCfg.Settings)))
	}

	if updated == sectionUpdates || isUpdatesChanged(oldCfg, newCfg) {
		errs = append(errs, applied("updates", lifecycle.UpdatesUpdateSignal.TryEmit(ctx, newCfg.Updates)))
	}

	if updated == sectionCluster || isSharedClusterChanged(oldCfg, newCfg) {
		errs = append(errs, applied("cluster", lifecycle.SharedClusterUpdateSignal.TryEmit(ctx, newCfg.Cluster)))
	}

	return errors.Join(errs...)
}

// applied names the section whose listeners returned err.
func applied(section string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("Failed to apply the %s config: %w", section, err)
}
