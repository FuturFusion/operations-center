package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_isSharedSettingsChanged(t *testing.T) {
	t.Parallel()

	newConfig := func(settings system.SettingsGlobalPut) SharedConfig {
		return SharedConfig{
			Settings: system.SettingsGlobal{
				SettingsGlobalPut: settings,
			},
		}
	}

	base := system.SettingsGlobalPut{
		ServerRegistrationScriptlet: "def register(): pass",
	}

	tests := []struct {
		// field is the field of system.SettingsGlobalPut the case covers.
		field  string
		mutate func(settings *system.SettingsGlobalPut)
	}{
		{
			field: "ServerRegistrationScriptlet",
			mutate: func(settings *system.SettingsGlobalPut) {
				settings.ServerRegistrationScriptlet = "def register(): fail"
			},
		},
	}

	require.False(t, isSharedSettingsChanged(newConfig(base), newConfig(base)), "identical settings are not reported as changed")

	covered := make([]string, 0, len(tests))

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			changed := base
			tc.mutate(&changed)

			require.True(t, isSharedSettingsChanged(newConfig(base), newConfig(changed)), "a change of %q must be reported", tc.field)
		})

		covered = append(covered, tc.field)
	}

	settingsType := reflect.TypeOf(system.SettingsGlobalPut{})
	fields := make([]string, 0, settingsType.NumField())

	for i := range settingsType.NumField() {
		fields = append(fields, settingsType.Field(i).Name)
	}

	require.ElementsMatch(t, fields, covered, "a field added to system.SettingsGlobalPut needs a case here, so it is known to be covered by isSharedSettingsChanged")
}

func Test_isMetricsChanged(t *testing.T) {
	tests := []struct {
		name        string
		oldSettings system.SettingsLocalPut
		newSettings system.SettingsLocalPut

		want bool
	}{
		{
			name:        "equal",
			oldSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},

			want: false,
		},
		{
			name:        "nil and empty components",
			oldSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: nil},
			newSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: []string{}},

			want: false,
		},
		{
			name:        "other setting differs",
			oldSettings: system.SettingsLocalPut{LogLevel: "WARN", MetricsEnabled: true, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsLocalPut{LogLevel: "DEBUG", MetricsEnabled: true, MetricsComponents: []string{"runtime"}},

			want: false,
		},
		{
			name:        "only enabled differs",
			oldSettings: system.SettingsLocalPut{MetricsEnabled: false, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},

			want: true,
		},
		{
			name:        "only components differ",
			oldSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: []string{"runtime"}},
			newSettings: system.SettingsLocalPut{MetricsEnabled: true, MetricsComponents: []string{"runtime", "api"}},

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			oldCfg := localConfig{Settings: system.SettingsLocal{SettingsLocalPut: tc.oldSettings}}
			newCfg := localConfig{Settings: system.SettingsLocal{SettingsLocalPut: tc.newSettings}}

			// Run test
			got := isMetricsChanged(oldCfg, newCfg)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
