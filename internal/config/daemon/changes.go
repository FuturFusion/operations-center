package config

import (
	"maps"
	"reflect"
	"slices"

	"github.com/FuturFusion/operations-center/internal/security/acme"
)

// This file is the single home for "what changed between two configurations".
// Validation, delegated validation and notification all ask these predicates,
// so the comparison of a given setting exists exactly once.
//
// All predicates take the committed configuration as oldCfg and the normalized
// candidate as newCfg, which is what makes them agree with what is persisted.

func isNetworkChanged(oldCfg, newCfg config) bool {
	return oldCfg.Network.NetworkPut != newCfg.Network.NetworkPut
}

func isOIDCOrOpenFGAChanged(oldCfg, newCfg config) bool {
	return oldCfg.Security.OIDC != newCfg.Security.OIDC || oldCfg.Security.OpenFGA != newCfg.Security.OpenFGA
}

func isTrustedTLSClientsChanged(oldCfg, newCfg config) bool {
	return !slices.Equal(oldCfg.Security.TrustedTLSClientCertFingerprints, newCfg.Security.TrustedTLSClientCertFingerprints) ||
		!slices.Equal(oldCfg.Security.TrustedTLSClientCertificates, newCfg.Security.TrustedTLSClientCertificates)
}

// isSecurityAuthChanged reports whether anything the authenticators or authorizers
// are built from has changed.
func isSecurityAuthChanged(oldCfg, newCfg config) bool {
	return isTrustedTLSClientsChanged(oldCfg, newCfg) || isOIDCOrOpenFGAChanged(oldCfg, newCfg)
}

func isTrustedHTTPSProxiesChanged(oldCfg, newCfg config) bool {
	return !slices.Equal(oldCfg.Security.TrustedHTTPSProxies, newCfg.Security.TrustedHTTPSProxies)
}

func isACMEChanged(oldCfg, newCfg config) bool {
	return acme.ACMEConfigChanged(oldCfg.Security.ACME, newCfg.Security.ACME)
}

func isLogLevelChanged(oldCfg, newCfg config) bool {
	return oldCfg.Settings.LogLevel != newCfg.Settings.LogLevel
}

func isLogLevelsChanged(oldCfg, newCfg config) bool {
	return !maps.Equal(oldCfg.Settings.LogLevels, newCfg.Settings.LogLevels)
}

func isPprofEnabledChanged(oldCfg, newCfg config) bool {
	return oldCfg.Settings.PprofEnabled != newCfg.Settings.PprofEnabled
}

func isSettingsChanged(oldCfg, newCfg config) bool {
	return !reflect.DeepEqual(oldCfg.Settings.SettingsPut, newCfg.Settings.SettingsPut)
}

func isUpdatesChanged(oldCfg, newCfg config) bool {
	return oldCfg.Updates.UpdatesPut != newCfg.Updates.UpdatesPut
}
