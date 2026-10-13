// Package component holds the naming rules for components. A component
// identifies a part of the system, e.g. for log filtering or for metrics.
//
// Component names are hierarchical and the levels are separated by dots, e.g.
// "provisioning.server_repo". A configuration for a component applies to all
// of its children as well.
package component

import (
	"fmt"
	"iter"
	"regexp"
	"slices"
	"strings"
)

// nameRegexp matches a valid component name: dot separated levels of lower
// case alphanumeric characters and underscores.
var nameRegexp = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)

// ValidateName checks the given component name for valid syntax.
func ValidateName(name string) error {
	if !nameRegexp.MatchString(name) {
		return fmt.Errorf("Component %q is invalid, must be dot separated levels of lower case alphanumeric characters and underscores", name)
	}

	return nil
}

// UnknownNames returns the given names, which neither are one of the known
// component names nor a parent of one, in the given order. Nothing is reported
// while no component is known.
func UnknownNames(known []string, names []string) []string {
	if len(known) == 0 {
		return nil
	}

	var unknown []string

	for _, name := range names {
		prefix := name + "."

		isKnown := slices.ContainsFunc(known, func(component string) bool {
			return component == name || strings.HasPrefix(component, prefix)
		})

		if !isKnown {
			unknown = append(unknown, name)
		}
	}

	return unknown
}

// Lineage returns the given component name followed by the names of all of its
// parents, the most specific name first.
func Lineage(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for name != "" {
			if !yield(name) {
				return
			}

			idx := strings.LastIndexByte(name, '.')
			if idx < 0 {
				return
			}

			name = name[:idx]
		}
	}
}
