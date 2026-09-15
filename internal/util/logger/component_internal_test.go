package logger

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLevelSetLevelFor(t *testing.T) {
	ls := newLevelSet(slog.LevelWarn, map[string]slog.Level{
		"provisioning":             slog.LevelDebug,
		"provisioning.server_repo": LevelTrace,
		"api":                      slog.LevelInfo,
	})

	tests := []struct {
		name      string
		component Component

		want slog.Level
	}{
		{
			name:      "exact match",
			component: "api",
			want:      slog.LevelInfo,
		},
		{
			name:      "most specific configuration wins",
			component: "provisioning.server_repo",
			want:      LevelTrace,
		},
		{
			name:      "parent applies to child",
			component: "provisioning.server_service",
			want:      slog.LevelDebug,
		},
		{
			name:      "parent applies to grandchild",
			component: "provisioning.adapter.scriptlet.server_port",
			want:      slog.LevelDebug,
		},
		{
			name:      "prefix only matches at a level boundary",
			component: "provisioningx.server_repo",
			want:      slog.LevelWarn,
		},
		{
			name:      "unconfigured component falls back to the default level",
			component: "inventory.image_service",
			want:      slog.LevelWarn,
		},
		{
			name:      "empty component falls back to the default level",
			component: "",
			want:      slog.LevelWarn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ls.levelFor(tc.component), "unexpected level for component %q", tc.component)
		})
	}
}

func TestLevelSetMin(t *testing.T) {
	require.Equal(t, slog.LevelWarn, newLevelSet(slog.LevelWarn, nil).min, "without overrides the minimum is the default level")
	require.Equal(t, LevelTrace, newLevelSet(slog.LevelWarn, map[string]slog.Level{"api": LevelTrace}).min, "an override more verbose than the default lowers the minimum")
	require.Equal(t, slog.LevelWarn, newLevelSet(slog.LevelWarn, map[string]slog.Level{"api": slog.LevelError}).min, "an override less verbose than the default keeps the minimum")
}

func TestLevelSetEnabled(t *testing.T) {
	ctx := ContextWithComponent(t.Context(), "provisioning.server_repo")

	withoutOverrides := newLevelSet(slog.LevelWarn, nil)
	require.False(t, withoutOverrides.enabled(ctx, slog.LevelDebug), "without overrides the component is ignored")
	require.True(t, withoutOverrides.enabled(ctx, slog.LevelWarn), "without overrides the default level applies")

	withOverrides := newLevelSet(slog.LevelWarn, map[string]slog.Level{"provisioning": slog.LevelDebug})
	require.True(t, withOverrides.enabled(ctx, slog.LevelDebug), "the component is enabled by the override of its parent")
	require.False(t, withOverrides.enabled(ctx, LevelTrace), "the override does not enable levels below itself")
	require.False(t, withOverrides.enabled(t.Context(), slog.LevelDebug), "an unscoped context falls back to the default level")
}

func TestComponentLevelFiltering(t *testing.T) {
	logBuf := &bytes.Buffer{}

	err := InitLogger(logBuf, "", false, false, true)
	require.NoError(t, err, "logger initialization must not fail")

	t.Cleanup(func() {
		require.NoError(t, InitLogger(logBuf, "", false, false, true), "the component levels must not leak into other tests")
	})

	provisioning := ContextWithComponent(t.Context(), "provisioning.server_repo")
	inventory := ContextWithComponent(t.Context(), "inventory.image_repo")

	slog.DebugContext(provisioning, "before override")
	require.Empty(t, logBuf.String(), "the default level WARN suppresses debug records")

	err = SetComponentLevels(map[string]slog.Level{"provisioning": LevelTrace})
	require.NoError(t, err, "setting the component levels must not fail")

	slog.Log(provisioning, LevelTrace, "provisioning record")
	slog.DebugContext(inventory, "inventory record")

	require.Contains(t, logBuf.String(), "provisioning record", "the component override raises the verbosity for the component")
	require.Contains(t, logBuf.String(), componentContextKey+"=provisioning.server_repo", "the component is recorded as log attribute")
	require.NotContains(t, logBuf.String(), "inventory record", "the component override does not affect other components")

	require.Contains(t, logBuf.String(), levelTraceStringShort, "the trace level is rendered by name")
}

func TestRegisterComponent(t *testing.T) {
	component := RegisterComponent("test.register_component")

	require.Equal(t, Component("test.register_component"), component, "RegisterComponent returns the registered component")
	require.Contains(t, Components(), component, "the registered component is reported by Components")

	require.NotPanics(t, func() { RegisterComponent("test.register_component") },
		"registering the same component twice is allowed, several packages can log under one component")

	require.Panics(t, func() { RegisterComponent("Test.Register_Component") },
		"an upper case name could never be configured, so it is rejected at registration")
	require.Panics(t, func() { RegisterComponent("test.register-component") },
		"a hyphen, as it can occur in a package name, could never be configured")
	require.Panics(t, func() { RegisterComponent("") },
		"an empty name could never be configured")
}

func TestDetachedContextKeepsComponent(t *testing.T) {
	ctx := ContextWithComponent(ContextWithAttr(t.Context(), slog.String("request_id", "1")), "api")

	component, ok := ComponentFromContext(DetachedContext(ctx))

	require.True(t, ok, "the detached context keeps the component")
	require.Equal(t, Component("api"), component, "the detached context keeps the component unchanged")
}

func TestValidateComponentLevels(t *testing.T) {
	tests := []struct {
		name            string
		componentLevels map[string]string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "empty",
			componentLevels: nil,
			assertErr:       require.NoError,
		},
		{
			name: "valid",
			componentLevels: map[string]string{
				"api":                      "INFO",
				"provisioning":             "DEBUG",
				"provisioning.server_repo": "TRACE",
			},
			assertErr: require.NoError,
		},
		{
			name:            "invalid level",
			componentLevels: map[string]string{"api": "LOUD"},
			assertErr:       require.Error,
		},
		{
			name:            "empty level",
			componentLevels: map[string]string{"api": ""},
			assertErr:       require.Error,
		},
		{
			name:            "upper case component name",
			componentLevels: map[string]string{"API": "INFO"},
			assertErr:       require.Error,
		},
		{
			name:            "trailing dot in component name",
			componentLevels: map[string]string{"api.": "INFO"},
			assertErr:       require.Error,
		},
		{
			name:            "empty component name",
			componentLevels: map[string]string{"": "INFO"},
			assertErr:       require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assertErr(t, ValidateComponentLevels(tc.componentLevels))
		})
	}
}

func TestParseComponentLevels(t *testing.T) {
	require.Nil(t, ParseComponentLevels(nil), "an empty configuration parses to no overrides")
	require.Equal(t,
		map[string]slog.Level{"api": slog.LevelInfo, "provisioning": LevelTrace},
		ParseComponentLevels(map[string]string{"api": "INFO", "provisioning": "TRACE"}),
		"the string representations are converted to slog levels",
	)
}

func TestUnknownComponents(t *testing.T) {
	require.Nil(t, filterUnknownComponents(nil, []string{"nowhere"}),
		"nothing is reported while no component is registered")

	RegisterComponent("test.unknown_components.child")

	require.Nil(t, unknownComponents([]string{
		"test",
		"test.unknown_components",
		"test.unknown_components.child",
	}), "a registered component and its parents are known")

	require.Equal(t, []string{"nowhere", "test.unknown_components.other"},
		unknownComponents([]string{
			"nowhere",
			"test.unknown_components.child",
			"test.unknown_components.other",
		}), "components which match nothing registered are reported")
}

func TestComponentLevelFilteringWithGroup(t *testing.T) {
	logBuf := &bytes.Buffer{}

	err := InitLogger(logBuf, "", false, false, true)
	require.NoError(t, err, "logger initialization must not fail")

	t.Cleanup(func() {
		require.NoError(t, InitLogger(logBuf, "", false, false, true), "the component levels must not leak into other tests")
	})

	grouped := slog.Default().WithGroup("group")

	provisioning := ContextWithComponent(t.Context(), "provisioning.server_repo")
	inventory := ContextWithComponent(t.Context(), "inventory.image_repo")

	grouped.DebugContext(provisioning, "before override")
	require.Empty(t, logBuf.String(), "a grouped logger is filtered by the default log level like any other")

	err = SetComponentLevels(map[string]slog.Level{"provisioning": slog.LevelDebug})
	require.NoError(t, err, "setting the component levels must not fail")

	grouped.DebugContext(provisioning, "provisioning record")
	grouped.DebugContext(inventory, "inventory record")

	out := logBuf.String()

	require.Contains(t, out, "provisioning record", "the component override reaches a grouped logger")
	require.NotContains(t, out, "inventory record", "a grouped logger does not bypass the filtering of the other components")
}

func TestValidateComponentLevelsKnown(t *testing.T) {
	RegisterComponent("test.validate_known.child")

	require.NoError(t, ValidateComponentLevelsKnown(nil), "an empty configuration has nothing unknown in it")
	require.NoError(t, ValidateComponentLevelsKnown(map[string]string{"test.validate_known": "DEBUG"}),
		"a parent of a registered component is known")

	err := ValidateComponentLevelsKnown(map[string]string{"test.validate_known.other": "DEBUG"})

	require.ErrorContains(t, err, `"test.validate_known.other"`, "the rejection names the unknown component")
}
