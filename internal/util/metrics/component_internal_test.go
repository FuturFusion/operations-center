package metrics

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestSetEnabledComponents(t *testing.T) {
	tests := []struct {
		name      string
		component string
		initial   []string
		enabled   []string

		want bool
	}{
		{
			name:      "nothing enabled",
			component: "test.set_enabled.child",
			initial:   []string{"test.set_enabled.child"},
			enabled:   nil,

			want: false,
		},
		{
			name:      "exact name",
			component: "test.set_enabled.child",
			initial:   nil,
			enabled:   []string{"test.set_enabled.child"},

			want: true,
		},
		{
			name:      "direct parent",
			component: "test.set_enabled.child",
			initial:   nil,
			enabled:   []string{"test.set_enabled"},

			want: true,
		},
		{
			name:      "top level parent",
			component: "test.set_enabled.child",
			initial:   nil,
			enabled:   []string{"other", "test"},

			want: true,
		},
		{
			name:      "child does not enable the parent",
			component: "test.set_enabled",
			initial:   []string{"test.set_enabled"},
			enabled:   []string{"test.set_enabled.child"},

			want: false,
		},
		{
			name:      "prefix without dot boundary",
			component: "test.set_enabled.child",
			initial:   []string{"test.set_enabled.child"},
			enabled:   []string{"test.set_ena"},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			component := RegisterComponent(tc.component)
			t.Cleanup(func() { SetEnabledComponents(nil) })

			SetEnabledComponents(tc.initial)
			require.NotEqual(t, tc.want, component.Enabled(), "the test starts in the opposite state")

			// Run test
			SetEnabledComponents(tc.enabled)

			// Assert
			require.Equal(t, tc.want, component.Enabled())
		})
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		enabled bool
		names   []string

		want bool
	}{
		{
			name:    "enabled and listed",
			initial: nil,
			enabled: true,
			names:   []string{"test.apply"},

			want: true,
		},
		{
			name:    "enabled and not listed",
			initial: []string{"test.apply"},
			enabled: true,
			names:   []string{"other"},

			want: false,
		},
		{
			name:    "disabled and listed",
			initial: []string{"test.apply"},
			enabled: false,
			names:   []string{"test.apply"},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			component := RegisterComponent("test.apply")
			t.Cleanup(func() { SetEnabledComponents(nil) })

			SetEnabledComponents(tc.initial)
			require.NotEqual(t, tc.want, component.Enabled(), "the test starts in the opposite state")

			// Run test
			Apply(tc.enabled, tc.names)

			// Assert
			require.Equal(t, tc.want, component.Enabled())
		})
	}
}

// registerComponentRun makes the component names unique, so a repeated test
// run registers new components.
var registerComponentRun atomic.Int64

func TestRegisterComponent(t *testing.T) {
	tests := []struct {
		name    string
		enabled []string

		wantEnabled bool
	}{
		{
			name:    "parent enabled before the registration",
			enabled: []string{"test.register_component"},

			wantEnabled: true,
		},
		{
			name:    "parent not enabled",
			enabled: []string{"other"},

			wantEnabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			name := fmt.Sprintf("test.register_component.late_%d", registerComponentRun.Add(1))

			SetEnabledComponents(tc.enabled)
			t.Cleanup(func() { SetEnabledComponents(nil) })

			// Run test
			component := RegisterComponent(name)

			// Assert
			require.Equal(t, tc.wantEnabled, component.Enabled(), "a component registered after the configuration is applied follows its parent")
			require.Equal(t, name, component.Name())
			require.Same(t, component, RegisterComponent(name), "registering the same name twice returns the same component")
			require.Contains(t, Components(), name)
			require.Contains(t, Components(), "runtime", "the runtime component is always registered")
		})
	}
}

func TestRegisterComponent_invalidName(t *testing.T) {
	tests := []struct {
		name          string
		componentName string
	}{
		{
			name:          "upper case",
			componentName: "Test.Register_Component",
		},
		{
			name:          "empty",
			componentName: "",
		},
		{
			name:          "trailing dot",
			componentName: "test.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Panics(t, func() { RegisterComponent(tc.componentName) }, "a name that can not be configured is rejected at registration")
		})
	}
}

func TestValidateComponents(t *testing.T) {
	tests := []struct {
		name  string
		names []string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:      "empty",
			names:     nil,
			assertErr: require.NoError,
		},
		{
			name:      "valid names",
			names:     []string{"runtime", "provisioning.server_service"},
			assertErr: require.NoError,
		},
		{
			name:      "upper case",
			names:     []string{"Runtime"},
			assertErr: require.Error,
		},
		{
			name:      "trailing dot",
			names:     []string{"runtime."},
			assertErr: require.Error,
		},
		{
			name:      "empty name",
			names:     []string{""},
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assertErr(t, ValidateComponents(tc.names))
		})
	}
}

func TestValidateComponentsKnown(t *testing.T) {
	RegisterComponent("test.validate_known.child")

	tests := []struct {
		name  string
		names []string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:      "empty",
			names:     nil,
			assertErr: require.NoError,
		},
		{
			name:      "registered component",
			names:     []string{"runtime", "test.validate_known.child"},
			assertErr: require.NoError,
		},
		{
			name:      "parent of a registered component",
			names:     []string{"test.validate_known"},
			assertErr: require.NoError,
		},
		{
			name:      "unknown component",
			names:     []string{"runtime", "test.validate_known.other"},
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assertErr(t, ValidateComponentsKnown(tc.names))
		})
	}
}

func TestGatedCollector(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		enabled []string

		wantFamilies int
	}{
		{
			name:    "enabled",
			initial: nil,
			enabled: []string{"test.gated_collector"},

			wantFamilies: 1,
		},
		{
			name:    "disabled",
			initial: []string{"test.gated_collector"},
			enabled: nil,

			wantFamilies: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			component := RegisterComponent("test.gated_collector")
			t.Cleanup(func() { SetEnabledComponents(nil) })

			gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_gated_collector"})
			reg := prometheus.NewRegistry()
			require.NoError(t, reg.Register(GatedCollector(component, gauge)))

			SetEnabledComponents(tc.initial)

			// Run test
			SetEnabledComponents(tc.enabled)
			families, err := reg.Gather()

			// Assert
			require.NoError(t, err)
			require.Len(t, families, tc.wantFamilies)
		})
	}
}

func TestGatedCollector_nilComponent(t *testing.T) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_gated_collector_nil"})

	require.Panics(t, func() { GatedCollector(nil, gauge) }, "a nil component fails at construction and not on the first scrape")
}

func TestRuntimeComponent(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		enabled []string

		assertNames func(t *testing.T, names []string)
	}{
		{
			name:    "enabled",
			initial: nil,
			enabled: []string{"runtime"},

			assertNames: func(t *testing.T, names []string) {
				t.Helper()

				require.Subset(t, names, []string{"go_goroutines", "operations_center_build_info", "operations_center_uptime_seconds"})
			},
		},
		{
			name:    "disabled",
			initial: []string{"runtime"},
			enabled: nil,

			assertNames: func(t *testing.T, names []string) {
				t.Helper()

				require.Empty(t, names, "a disabled runtime component exposes no metrics")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			t.Cleanup(func() { SetEnabledComponents(nil) })
			SetEnabledComponents(tc.initial)

			// Run test
			SetEnabledComponents(tc.enabled)
			families, err := Gatherer().Gather()

			// Assert
			require.NoError(t, err)

			names := make([]string, 0, len(families))
			for _, family := range families {
				names = append(names, family.GetName())
			}

			tc.assertNames(t, names)
		})
	}
}
