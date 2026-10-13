package metrics

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/FuturFusion/operations-center/internal/util/component"
)

// Component enables or disables a group of metrics.
//
// Component names are hierarchical. Dots separate the levels, for example
// "provisioning.server_service". Enabling a component also enables all of its
// children.
type Component struct {
	name    string
	enabled atomic.Bool
}

var (
	componentsMu sync.Mutex
	components   = map[string]*Component{}

	// Keep enabledNames to enable components registered after the
	// configuration is applied.
	enabledNames []string
)

// RegisterComponent registers the component with the given name and returns it.
// Registering the same name again returns the same component.
//
// It is meant to be called from package level variable declarations, so the set
// of known components is complete before any configuration is applied.
func RegisterComponent(name string) *Component {
	err := component.ValidateName(name)
	if err != nil {
		panic(err)
	}

	componentsMu.Lock()
	defer componentsMu.Unlock()

	c, ok := components[name]
	if ok {
		return c
	}

	c = &Component{name: name}
	c.enabled.Store(isEnabled(name, enabledNames))
	components[name] = c

	return c
}

// Components returns the names of all registered components in lexical order.
func Components() []string {
	componentsMu.Lock()
	defer componentsMu.Unlock()

	return slices.Sorted(maps.Keys(components))
}

// Name returns the name of the component.
func (c *Component) Name() string {
	return c.name
}

// Enabled reports whether the component collects its metrics.
func (c *Component) Enabled() bool {
	return c.enabled.Load()
}

// Apply applies the metrics settings. No component is enabled if enabled is
// false.
func Apply(enabled bool, names []string) {
	if !enabled {
		names = nil
	}

	SetEnabledComponents(names)
}

// SetEnabledComponents enables the components with the given names and their
// children. All other components are disabled.
func SetEnabledComponents(names []string) {
	componentsMu.Lock()
	defer componentsMu.Unlock()

	enabledNames = slices.Clone(names)

	for name, c := range components {
		c.enabled.Store(isEnabled(name, enabledNames))
	}
}

// isEnabled reports whether name or one of its parents is in enabled.
func isEnabled(name string, enabled []string) bool {
	for candidate := range component.Lineage(name) {
		if slices.Contains(enabled, candidate) {
			return true
		}
	}

	return false
}

// ValidateComponents checks the given component names for valid syntax.
func ValidateComponents(names []string) error {
	for _, name := range names {
		err := component.ValidateName(name)
		if err != nil {
			return err
		}
	}

	return nil
}

// ValidateComponentsKnown checks the given component names against the
// registered components. A name is known if it is a registered component or a
// parent of one.
func ValidateComponentsKnown(names []string) error {
	unknown := component.UnknownNames(Components(), names)
	if len(unknown) == 0 {
		return nil
	}

	return fmt.Errorf("Metrics enabled for unknown components %q", strings.Join(unknown, ","))
}

type gatedCollector struct {
	component *Component
	inner     prometheus.Collector
}

// GatedCollector returns a collector for the metrics of inner. It collects
// nothing while the component is disabled. It panics if the component is nil.
func GatedCollector(c *Component, inner prometheus.Collector) prometheus.Collector {
	if c == nil {
		panic("metrics: GatedCollector requires a component")
	}

	return gatedCollector{
		component: c,
		inner:     inner,
	}
}

// Describe implements prometheus.Collector.
func (g gatedCollector) Describe(ch chan<- *prometheus.Desc) {
	g.inner.Describe(ch)
}

// Collect implements prometheus.Collector.
func (g gatedCollector) Collect(ch chan<- prometheus.Metric) {
	if !g.component.Enabled() {
		return
	}

	g.inner.Collect(ch)
}
