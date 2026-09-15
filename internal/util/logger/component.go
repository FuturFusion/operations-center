package logger

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

const componentContextKey = "component"

// Component identifies a part of the system for the purpose of log filtering.
//
// Component names are hierarchical and the levels are separated by dots, e.g.
// "provisioning.server_repo". A log level configured for a component applies to
// all of its children as well.
type Component string

var (
	componentsMu sync.Mutex
	components   = map[Component]struct{}{}
)

// RegisterComponent registers the component with the given name and returns it.
//
// It is meant to be called from package level variable declarations, so the set
// of known components is complete before any configuration is applied.
func RegisterComponent(name string) Component {
	err := ValidateComponent(name)
	if err != nil {
		panic(err)
	}

	componentsMu.Lock()
	defer componentsMu.Unlock()

	component := Component(name)
	components[component] = struct{}{}

	return component
}

// Components returns all registered components in lexical order.
func Components() []Component {
	componentsMu.Lock()
	defer componentsMu.Unlock()

	return slices.Sorted(maps.Keys(components))
}

// componentNameRegexp matches a valid component name: dot separated levels of
// lower case alphanumeric characters and underscores.
var componentNameRegexp = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)

// ValidateComponent checks the given component name for valid syntax.
func ValidateComponent(name string) error {
	if !componentNameRegexp.MatchString(name) {
		return fmt.Errorf("Component %q is invalid, must be dot separated levels of lower case alphanumeric characters and underscores", name)
	}

	return nil
}

// ValidateComponentLevels checks the given log levels per component. The keys
// are component names, the values are string representations of a log level.
func ValidateComponentLevels(componentLevels map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(componentLevels)) {
		err := ValidateComponent(name)
		if err != nil {
			return err
		}

		if componentLevels[name] == "" {
			return fmt.Errorf("Log level for component %q can not be empty", name)
		}

		err = ValidateLevel(componentLevels[name])
		if err != nil {
			return err
		}
	}

	return nil
}

// ParseComponentLevels converts the string representations of the given log
// levels per component into actual slog.Level values.
func ParseComponentLevels(componentLevels map[string]string) map[string]slog.Level {
	if len(componentLevels) == 0 {
		return nil
	}

	parsed := make(map[string]slog.Level, len(componentLevels))
	for name, levelStr := range componentLevels {
		parsed[name] = ParseLevel(levelStr)
	}

	return parsed
}

// ValidateComponentLevelsKnown checks the given component names against the
// registered components. A name is known if it is a registered component or a
// parent of one.
func ValidateComponentLevelsKnown(componentLevels map[string]string) error {
	unknown := unknownComponents(slices.Sorted(maps.Keys(componentLevels)))
	if len(unknown) == 0 {
		return nil
	}

	return fmt.Errorf("Log levels configured for unknown components %q", strings.Join(unknown, ","))
}

// unknownComponents returns the given component names, which neither are a
// registered component nor a parent of one, in the given order.
func unknownComponents(names []string) []string {
	return filterUnknownComponents(Components(), names)
}

func filterUnknownComponents(known []Component, names []string) []string {
	if len(known) == 0 {
		return nil
	}

	var unknown []string

	for _, name := range names {
		prefix := name + "."

		isKnown := slices.ContainsFunc(known, func(component Component) bool {
			return string(component) == name || strings.HasPrefix(string(component), prefix)
		})

		if !isKnown {
			unknown = append(unknown, name)
		}
	}

	return unknown
}

type componentKey struct{}

// ContextWithComponent returns a copy of parent, which is scoped to the given
// component. Log records emitted with the returned context are attributed to
// the component and are filtered using the log level configured for it.
func ContextWithComponent(parent context.Context, component Component) context.Context {
	return context.WithValue(parent, componentKey{}, component)
}

// ComponentFromContext returns the component the given context is scoped to.
func ComponentFromContext(ctx context.Context) (Component, bool) {
	component, ok := ctx.Value(componentKey{}).(Component)

	return component, ok
}

// levelSet holds the log levels currently in effect. It is only ever replaced
// as a whole, so readers never observe a partially updated state.
type levelSet struct {
	// defaultLevel applies to every component without a more specific
	// configuration.
	defaultLevel slog.Level

	// overrides holds the log levels configured for individual components. The
	// keys are component names or prefixes thereof.
	overrides map[string]slog.Level

	// min is the most verbose level in effect anywhere. It drives the handler
	// properties, which can not be decided per record.
	min slog.Level
}

// levels holds the log levels in effect. It is kept separate from the handler,
// so a level change takes effect for loggers derived from the default logger
// earlier, without the handler having to be rebuilt.
var levels atomic.Pointer[levelSet]

func init() {
	levels.Store(newLevelSet(slog.LevelWarn, nil))
}

func newLevelSet(defaultLevel slog.Level, overrides map[string]slog.Level) *levelSet {
	ls := &levelSet{
		defaultLevel: defaultLevel,
		overrides:    maps.Clone(overrides),
		min:          defaultLevel,
	}

	for _, level := range ls.overrides {
		ls.min = min(ls.min, level)
	}

	return ls
}

// levelFor returns the log level in effect for the given component. The most
// specific configuration wins, a level configured for a parent applies to all
// of its children.
func (ls *levelSet) levelFor(component Component) slog.Level {
	name := string(component)

	for name != "" {
		level, ok := ls.overrides[name]
		if ok {
			return level
		}

		idx := strings.LastIndexByte(name, '.')
		if idx < 0 {
			break
		}

		name = name[:idx]
	}

	return ls.defaultLevel
}

// enabled reports whether a record of the given level, emitted with the given
// context, should be handled.
func (ls *levelSet) enabled(ctx context.Context, level slog.Level) bool {
	// Without any override, the component of the record is irrelevant, so the
	// lookup in the context is skipped.
	if len(ls.overrides) == 0 {
		return level >= ls.defaultLevel
	}

	component, _ := ComponentFromContext(ctx)

	return level >= ls.levelFor(component)
}
