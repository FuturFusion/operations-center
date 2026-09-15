package config

import (
	"context"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// notify informs the subsystems about a configuration that has already been
// persisted and committed. It can not fail the update anymore, so everything
// that may reject a configuration belongs in validate or validateDelegated and
// failures here are logged only.
//
// The payloads are taken from newCfg, the committed configuration, so listeners
// see the same values as GetNetwork and friends, defaults included.
//
// The section the update was addressed to is always notified, even if the values
// did not change, so re-submitting a configuration applies it again. That is how
// an operator recovers a subsystem which failed to pick up an earlier update.
// Every other section is only notified if it actually changed, so an update of
// one section does not disturb the others.
func notify(ctx context.Context, updated section, oldCfg, newCfg config) {
	if updated == sectionNetwork || isNetworkChanged(oldCfg, newCfg) {
		lifecycle.NetworkUpdateSignal.Emit(ctx, newCfg.Network)
	}

	if isSecurityAuthChanged(oldCfg, newCfg) {
		lifecycle.SecurityUpdateSignal.Emit(ctx, newCfg.Security)
	}

	if isTrustedHTTPSProxiesChanged(oldCfg, newCfg) {
		lifecycle.SecurityTrustedHTTPSProxiesUpdateSignal.Emit(ctx, newCfg.Security.TrustedHTTPSProxies)
	}

	if isACMEChanged(oldCfg, newCfg) {
		lifecycle.SecurityACMEUpdateSignal.Emit(ctx, newCfg.Security.ACME)
	}

	if isLogLevelChanged(oldCfg, newCfg) {
		err := logger.SetLogLevel(logger.ParseLevel(newCfg.Settings.LogLevel))
		if err != nil {
			slog.ErrorContext(ctx, "Failed to apply log level from updated config", logger.Err(err))
		}
	}

	if isLogLevelsChanged(oldCfg, newCfg) {
		err := logger.SetComponentLevels(logger.ParseComponentLevels(newCfg.Settings.LogLevels))
		if err != nil {
			slog.ErrorContext(ctx, "Failed to apply per component log levels from updated config", logger.Err(err))
		}
	}

	if updated == sectionSettings || isSettingsChanged(oldCfg, newCfg) {
		err := lifecycle.SettingsUpdateSignal.TryEmit(ctx, newCfg.Settings)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to apply updated settings config", logger.Err(err))
		}
	}

	if updated == sectionUpdates || isUpdatesChanged(oldCfg, newCfg) {
		lifecycle.UpdatesUpdateSignal.Emit(ctx, newCfg.Updates)
	}
}
