package e2e

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ocState is the state of Operations Center, which scenarios change and their
// cleanups restore. It does not cover servers, clusters and updates. The pool
// manages the servers and the clusters. Operations Center refreshes the
// updates in the background.
type ocState struct {
	// configs maps the name of a system config to its JSON with sorted keys.
	configs map[string]string

	// objects maps the kind of an object to the sorted identifiers of the
	// objects.
	objects map[string][]string
}

// ocConfigs are the system configs in ocState. "show" returns the same fields,
// which "edit" accepts.
var ocConfigs = []string{"security", "settings", "updates", "network"}

// ocObjectKind is a kind of object in Operations Center, which scenarios
// create.
type ocObjectKind struct {
	name string

	// list prints the identifiers of the objects as a JSON array.
	list string

	// remove removes the object with the identifier given as second argument.
	remove string
}

// ocObjectKinds are the kinds of objects in ocState. The restore removes the
// objects in this order, e.g. the tokens before their channels.
var ocObjectKinds = []ocObjectKind{
	{
		name:   "token",
		list:   `../bin/operations-center.linux.%s provisioning token list -f json | jq -c '[ .[].uuid ]'`,
		remove: `../bin/operations-center.linux.%s provisioning token remove %s`,
	},
	{
		name:   "channel",
		list:   `../bin/operations-center.linux.%s provisioning channel list -f json | jq -c '[ .[].name ]'`,
		remove: `../bin/operations-center.linux.%s provisioning channel remove %s`,
	},
	{
		name:   "cluster template",
		list:   `../bin/operations-center.linux.%s provisioning cluster-template list -f json | jq -c '[ .[].name ]'`,
		remove: `../bin/operations-center.linux.%s provisioning cluster-template remove %s`,
	},
	{
		name:   "image source",
		list:   `../bin/operations-center.linux.%s image incus source list -f json | jq -c '[ .[].name ]'`,
		remove: `../bin/operations-center.linux.%s image incus source remove %s`,
	},
	{
		name:   "image",
		list:   `../bin/operations-center.linux.%s image incus list -f json | jq -c '[ .[].name ]'`,
		remove: `../bin/operations-center.linux.%s image incus remove %s`,
	},
}

func mustObserveOCState(t *testing.T) ocState {
	t.Helper()

	state := ocState{
		configs: map[string]string{},
		objects: map[string][]string{},
	}

	for _, name := range ocConfigs {
		resp := mustRun(t, `../bin/operations-center.linux.%s system %s show -f json | jq -cS .`, cpuArch, name)
		state.configs[name] = resp.OutputTrimmed()
	}

	for _, kind := range ocObjectKinds {
		resp := mustRun(t, kind.list, cpuArch)

		var ids []string
		err := json.Unmarshal([]byte(resp.OutputTrimmed()), &ids)
		require.NoError(t, err)

		slices.Sort(ids)
		state.objects[kind.name] = ids
	}

	return state
}

// ocStateDiff lists the differences between two states of Operations Center.
type ocStateDiff struct {
	configs []string
	added   map[string][]string
	removed map[string][]string
}

func diffOCState(reference ocState, current ocState) ocStateDiff {
	diff := ocStateDiff{
		added:   map[string][]string{},
		removed: map[string][]string{},
	}

	for _, name := range slices.Sorted(maps.Keys(reference.configs)) {
		if reference.configs[name] != current.configs[name] {
			diff.configs = append(diff.configs, name)
		}
	}

	for kind, ids := range reference.objects {
		for _, id := range current.objects[kind] {
			if !slices.Contains(ids, id) {
				diff.added[kind] = append(diff.added[kind], id)
			}
		}

		for _, id := range ids {
			if !slices.Contains(current.objects[kind], id) {
				diff.removed[kind] = append(diff.removed[kind], id)
			}
		}
	}

	return diff
}

func (d ocStateDiff) isEmpty() bool {
	return len(d.configs) == 0 && len(d.added) == 0 && len(d.removed) == 0
}

func (d ocStateDiff) String() string {
	lines := make([]string, 0, len(d.configs)+len(d.added)+len(d.removed))

	for _, name := range d.configs {
		lines = append(lines, fmt.Sprintf("changed system config %q", name))
	}

	for _, kind := range slices.Sorted(maps.Keys(d.added)) {
		lines = append(lines, fmt.Sprintf("added %s %v", kind, d.added[kind]))
	}

	for _, kind := range slices.Sorted(maps.Keys(d.removed)) {
		lines = append(lines, fmt.Sprintf("removed %s %v", kind, d.removed[kind]))
	}

	return strings.Join(lines, "\n")
}
