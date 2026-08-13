package cluster_test

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

// The version data, the fake servers report while passing through a rolling
// cluster update. Version "1" is installed initially, version "2" is available.
func versionData(osVersion string, osVersionNext string, needsReboot bool, incusVersion string, inMaintenance api.InMaintenanceState) api.ServerVersionData {
	return api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "incusos",
			Version:     osVersion,
			VersionNext: osVersionNext,
			NeedsReboot: needsReboot,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:          "incus",
				Version:       incusVersion,
				InMaintenance: inMaintenance,
			},
		},
		UpdateChannel: "stable",
	}
}

var (
	versionDataInitial    = versionData("1", "1", false, "1", api.NotInMaintenance)
	versionDataUpdating   = versionData("1", "1", false, "1", api.NotInMaintenance)
	versionDataUpdated    = versionData("1", "2", true, "2", api.NotInMaintenance)
	versionDataEvacuating = versionData("1", "2", true, "2", api.InMaintenanceEvacuating)
	versionDataEvacuated  = versionData("1", "2", true, "2", api.InMaintenanceEvacuated)
	versionDataRebooting  = versionData("2", "2", true, "2", api.InMaintenanceEvacuated)
	versionDataRebooted   = versionData("2", "2", false, "2", api.InMaintenanceEvacuated)
	versionDataRestoring  = versionData("2", "2", false, "2", api.InMaintenanceRestoring)
	versionDataRestored   = versionData("2", "2", false, "2", api.NotInMaintenance)
)

// serverWorld is the state of the fake servers, the ServerClientPortMock serves.
//
// State transitions, which a real server completes asynchronously, are released
// explicitly by the test.
type serverWorld struct {
	mu          sync.Mutex
	versionData map[string]api.ServerVersionData
	rebooting   map[string]bool
	pending     []serverWorldTransition
}

// serverWorldTransition is the state a server reaches once it has completed an
// action, that has been triggered on it.
type serverWorldTransition struct {
	server      string
	versionData api.ServerVersionData
	rebooting   bool

	// callback, if set, is invoked after the transition has been applied. It is
	// invoked without holding the lock, since it queries the server again.
	callback func(ctx context.Context, err error)
}

func newServerWorld(versionData map[string]api.ServerVersionData) *serverWorld {
	return &serverWorld{
		versionData: versionData,
		rebooting:   map[string]bool{},
	}
}

func (w *serverWorld) getVersionData(name string) api.ServerVersionData {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Return a copy, the caller enriches the version data with calculated fields.
	versionData := w.versionData[name]
	versionData.Applications = slices.Clone(versionData.Applications)

	return versionData
}

func (w *serverWorld) isRebooting(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.rebooting[name]
}

// set applies the state, a server reaches immediately when an action is triggered
// on it.
func (w *serverWorld) set(name string, versionData api.ServerVersionData, rebooting bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.versionData[name] = versionData
	w.rebooting[name] = rebooting
}

// deferTransition records the state, the server reaches once it has completed the action.
func (w *serverWorld) deferTransition(transition serverWorldTransition) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, transition)
}

// pendingCount returns the number of actions, that have been triggered but not yet
// completed.
func (w *serverWorld) pendingCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return len(w.pending)
}

// release completes the action, that has been triggered first.
func (w *serverWorld) release(ctx context.Context) {
	w.mu.Lock()

	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}

	transition := w.pending[0]
	w.pending = w.pending[1:]

	w.versionData[transition.server] = transition.versionData
	w.rebooting[transition.server] = transition.rebooting

	w.mu.Unlock()

	if transition.callback != nil {
		transition.callback(ctx, nil)
	}
}

var clusterUpdateStateRegexp = regexp.MustCompile(`\[\s*(\d+)/\s*(\d+)\]`)

// requireProgressOnlyMovesForward asserts, that the progress reported to the user
// never moves backwards and that the total number of steps stays constant.
func requireProgressOnlyMovesForward(t *testing.T, observed []string) {
	t.Helper()

	require.NotEmpty(t, observed)

	previousStep := 0
	previousTotalSteps := 0

	for i, description := range observed {
		if description == "" {
			continue
		}

		match := clusterUpdateStateRegexp.FindStringSubmatch(description)
		require.Len(t, match, 3, "observation %d (%q) does not report a step", i, description)

		step, err := strconv.Atoi(match[1])
		require.NoError(t, err)

		totalSteps, err := strconv.Atoi(match[2])
		require.NoError(t, err)

		require.GreaterOrEqual(t, step, previousStep, "progress moved backwards at observation %d, observed: %v", i, observed)

		if previousTotalSteps != 0 {
			require.Equal(t, previousTotalSteps, totalSteps, "total number of steps changed at observation %d, observed: %v", i, observed)
		}

		previousStep = step
		previousTotalSteps = totalSteps
	}

	require.NotZero(t, previousStep, "no progress observed at all, observed: %v", observed)
}

// dedupe removes consecutive duplicates and empty entries, so that a sequence of
// observations can be compared against the sequence of states, that is expected to
// be passed through.
func dedupe(in []string) []string {
	out := make([]string, 0, len(in))

	for _, value := range in {
		if value == "" {
			continue
		}

		if len(out) > 0 && out[len(out)-1] == value {
			continue
		}

		out = append(out, value)
	}

	return out
}

var clusterUpdateStateLogRegexp = regexp.MustCompile(`cluster_update_state=(?:"((?:[^"\\]|\\.)*)"|(\S+))`)

// clusterUpdateStatesFromLog extracts the sequence of cluster update states, that
// the control loop reported while acting on the cluster, with consecutive
// duplicates removed.
func clusterUpdateStatesFromLog(t *testing.T, logOutput string) []string {
	t.Helper()

	matches := clusterUpdateStateLogRegexp.FindAllStringSubmatch(logOutput, -1)

	states := make([]string, 0, len(matches))
	for _, match := range matches {
		value := match[1]
		if value == "" {
			value = match[2]
		}

		unquoted, err := strconv.Unquote(`"` + value + `"`)
		require.NoError(t, err)

		states = append(states, unquoted)
	}

	return dedupe(states)
}
