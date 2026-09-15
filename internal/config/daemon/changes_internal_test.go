package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_isSettingsChanged(t *testing.T) {
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
