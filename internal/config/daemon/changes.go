package config

import (
	"maps"
	"reflect"
	"slices"

	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// This file is the single home for "what changed between two configurations".
// Validation, delegated validation and notification all ask these predicates,
// so the comparison of a given setting exists exactly once.
//
// All predicates take the committed configuration as oldCfg and the normalized
// candidate as newCfg, which is what makes them agree with what is persisted.

func isNetworkChanged(oldCfg, newCfg localConfig) bool {
	return oldCfg.Network.NetworkPut != newCfg.Network.NetworkPut
}

func isSharedNetworkChanged(oldCfg, newCfg system.SharedConfig) bool {
	return oldCfg.Network.NetworkGlobalPut != newCfg.Network.NetworkGlobalPut
}

func isOIDCOrOpenFGAChanged(oldCfg, newCfg system.SharedConfig) bool {
	return oldCfg.Security.OIDC != newCfg.Security.OIDC || oldCfg.Security.OpenFGA != newCfg.Security.OpenFGA
}

func isTrustedTLSClientsChanged(oldCfg, newCfg system.SharedConfig) bool {
	return !slices.Equal(oldCfg.Security.TrustedTLSClientCertFingerprints, newCfg.Security.TrustedTLSClientCertFingerprints) ||
		!slices.Equal(oldCfg.Security.TrustedTLSClientCertificates, newCfg.Security.TrustedTLSClientCertificates)
}

// isSecurityAuthChanged reports whether anything the authenticators or authorizers
// are built from has changed.
func isSecurityAuthChanged(oldCfg, newCfg system.SharedConfig) bool {
	return isTrustedTLSClientsChanged(oldCfg, newCfg) || isOIDCOrOpenFGAChanged(oldCfg, newCfg)
}

func isTrustedHTTPSProxiesChanged(oldCfg, newCfg system.SharedConfig) bool {
	return !slices.Equal(oldCfg.Security.TrustedHTTPSProxies, newCfg.Security.TrustedHTTPSProxies)
}

func isACMEChanged(oldCfg, newCfg system.SharedConfig) bool {
	return acme.ACMEConfigChanged(oldCfg.Security.ACME, newCfg.Security.ACME)
}

func isLogLevelChanged(oldCfg, newCfg localConfig) bool {
	return oldCfg.Settings.LogLevel != newCfg.Settings.LogLevel
}

func isLogLevelsChanged(oldCfg, newCfg localConfig) bool {
	return !maps.Equal(oldCfg.Settings.LogLevels, newCfg.Settings.LogLevels)
}

func isPprofEnabledChanged(oldCfg, newCfg localConfig) bool {
	return oldCfg.Settings.PprofEnabled != newCfg.Settings.PprofEnabled
}

func isMetricsChanged(oldCfg, newCfg localConfig) bool {
	return oldCfg.Settings.MetricsEnabled != newCfg.Settings.MetricsEnabled ||
		!slices.Equal(oldCfg.Settings.MetricsComponents, newCfg.Settings.MetricsComponents)
}

func isSharedSettingsChanged(oldCfg, newCfg system.SharedConfig) bool {
	return !reflect.DeepEqual(oldCfg.Settings.SettingsGlobalPut, newCfg.Settings.SettingsGlobalPut)
}

func isSharedClusterChanged(oldCfg, newCfg system.SharedConfig) bool {
	return oldCfg.Cluster.ClusterPut != newCfg.Cluster.ClusterPut
}

func isUpdatesChanged(oldCfg, newCfg system.SharedConfig) bool {
	return oldCfg.Updates.UpdatesPut != newCfg.Updates.UpdatesPut
}
