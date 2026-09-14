package config

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/lifecycle"
)

// validateDelegated asks the subsystems owning a setting to validate it. The
// config package deliberately knows nothing about their rules; it only knows
// which sections to hand out and in which order.
//
// It runs with updateMu held, but no lock guards the committed configuration,
// so a listener is free to read it. Listeners must not write it, see the note
// on store.
func validateDelegated(ctx context.Context, oldCfg, newCfg config) error {
	err := lifecycle.UpdatesValidateSignal.TryEmit(ctx, newCfg.Updates)
	if err != nil {
		return err
	}

	err = lifecycle.SettingsValidateSignal.TryEmit(ctx, newCfg.Settings)
	if err != nil {
		return err
	}

	// Only emitted on change, since the connectivity probes performed during
	// validation should not get in the way of unrelated updates.
	if isOIDCOrOpenFGAChanged(oldCfg, newCfg) {
		err = lifecycle.SecurityValidateSignal.TryEmit(ctx, newCfg.Security)
		if err != nil {
			return err
		}
	}

	return nil
}
