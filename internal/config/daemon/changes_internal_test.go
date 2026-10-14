package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_isSettingsChanged(t *testing.T) {
	t.Parallel()

	newConfig := func(settings system.SettingsPut) config {
		return config{
			Settings: system.Settings{
				SettingsPut: settings,
			},
		}
	}

	base := system.SettingsPut{
		LogLevel:                    "WARN",
		LogLevels:                   map[string]string{"api": "DEBUG"},
		ServerRegistrationScriptlet: "def register(): pass",
	}

	tests := []struct {
		// field is the field of system.SettingsPut the case covers.
		field  string
		mutate func(settings *system.SettingsPut)
	}{
		{
			field:  "LogLevel",
			mutate: func(settings *system.SettingsPut) { settings.LogLevel = "DEBUG" },
		},
		{
			field:  "LogLevels",
			mutate: func(settings *system.SettingsPut) { settings.LogLevels = map[string]string{"api": "TRACE"} },
		},
		{
			field:  "ServerRegistrationScriptlet",
			mutate: func(settings *system.SettingsPut) { settings.ServerRegistrationScriptlet = "def register(): fail" },
		},
		{
			field:  "PprofEnabled",
			mutate: func(settings *system.SettingsPut) { settings.PprofEnabled = true },
		},
		{
			field:  "MetricsEnabled",
			mutate: func(settings *system.SettingsPut) { settings.MetricsEnabled = true },
		},
		{
			field:  "MetricsComponents",
			mutate: func(settings *system.SettingsPut) { settings.MetricsComponents = []string{"runtime"} },
		},
	}

	require.False(t, isSettingsChanged(newConfig(base), newConfig(base)), "identical settings are not reported as changed")

	covered := make([]string, 0, len(tests))

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			changed := base
			tc.mutate(&changed)

			require.True(t, isSettingsChanged(newConfig(base), newConfig(changed)), "a change of %q must be reported", tc.field)
		})

		covered = append(covered, tc.field)
	}

	settingsType := reflect.TypeOf(system.SettingsPut{})
	fields := make([]string, 0, settingsType.NumField())

	for i := range settingsType.NumField() {
		fields = append(fields, settingsType.Field(i).Name)
	}

	require.ElementsMatch(t, fields, covered, "a field added to system.SettingsPut needs a case here, so it is known to be covered by isSettingsChanged")
}

func Test_isMetricsChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		oldSettings system.SettingsPut
		newSettings system.SettingsPut

		want bool
	}{
		{
			name:        "equal",
			oldSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},

			want: false,
		},
		{
			name:        "nil and empty components",
			oldSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: nil},
			newSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: []string{}},

			want: false,
		},
		{
			name:        "other setting differs",
			oldSettings: system.SettingsPut{LogLevel: "WARN", MetricsEnabled: true, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsPut{LogLevel: "DEBUG", MetricsEnabled: true, MetricsComponents: []string{"runtime"}},

			want: false,
		},
		{
			name:        "only enabled differs",
			oldSettings: system.SettingsPut{MetricsEnabled: false, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},

			want: true,
		},
		{
			name:        "only components differ",
			oldSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsPut{MetricsEnabled: true, MetricsComponents: []string{"runtime", "api"}},

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			oldCfg := config{Settings: system.Settings{SettingsPut: tc.oldSettings}}
			newCfg := config{Settings: system.Settings{SettingsPut: tc.newSettings}}

			// Run test
			got := isMetricsChanged(oldCfg, newCfg)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_isSecurityAuthChanged(t *testing.T) {
	t.Parallel()

	newConfig := func(security system.SecurityPut) config {
		return config{
			Security: system.Security{
				SecurityPut: security,
			},
		}
	}

	tests := []struct {
		name   string
		mutate func(security *system.SecurityPut)

		want bool
	}{
		{
			name:   "unchanged",
			mutate: func(security *system.SecurityPut) {},

			want: false,
		},
		{
			name: "TrustedTLSClientCertFingerprints",
			mutate: func(security *system.SecurityPut) {
				security.TrustedTLSClientCertFingerprints = []string{"other"}
			},

			want: true,
		},
		{
			name: "TrustedTLSClientCertificates",
			mutate: func(security *system.SecurityPut) {
				security.TrustedTLSClientCertificates = []string{"other"}
			},

			want: true,
		},
		{
			name: "TrustedTLSMetricsClientCertFingerprints",
			mutate: func(security *system.SecurityPut) {
				security.TrustedTLSMetricsClientCertFingerprints = []string{"other"}
			},

			want: true,
		},
		{
			name: "TrustedTLSMetricsClientCertificates",
			mutate: func(security *system.SecurityPut) {
				security.TrustedTLSMetricsClientCertificates = []string{"other"}
			},

			want: true,
		},
		{
			name:   "OIDC",
			mutate: func(security *system.SecurityPut) { security.OIDC.Issuer = "https://issuer.example.com" },

			want: true,
		},
		{
			name:   "OpenFGA",
			mutate: func(security *system.SecurityPut) { security.OpenFGA.StoreID = "store" },

			want: true,
		},
		{
			name:   "TrustedHTTPSProxies",
			mutate: func(security *system.SecurityPut) { security.TrustedHTTPSProxies = []string{"127.0.0.1"} },

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			base := system.SecurityPut{
				TrustedTLSClientCertFingerprints:        []string{"fingerprint"},
				TrustedTLSClientCertificates:            []string{"certificate"},
				TrustedTLSMetricsClientCertFingerprints: []string{"metrics fingerprint"},
				TrustedTLSMetricsClientCertificates:     []string{"metrics certificate"},
			}

			changed := base
			tc.mutate(&changed)

			// Run test
			got := isSecurityAuthChanged(newConfig(base), newConfig(changed))

			// Assert
			require.Equal(t, tc.want, got, "a change of %q must be reported as %t", tc.name, tc.want)
		})
	}
}
