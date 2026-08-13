package cluster

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

// clusterUpdateSteps is the ordered sequence of per server update states, that a
// server passes through during a rolling cluster update. The position of a state
// in this slice defines how much of the work for the respective server is done,
// which is the basis for the progress reported to the user.
var clusterUpdateSteps = []api.ServerUpdateState{
	api.ServerUpdateStateUpdatePending,
	api.ServerUpdateStateUpdating,
	api.ServerUpdateStateEvacuationPending,
	api.ServerUpdateStateEvacuating,
	api.ServerUpdateStateInMaintenanceRebootPending,
	api.ServerUpdateStateInMaintenanceRebooting,
	api.ServerUpdateStateInMaintenanceRestorePending,
	api.ServerUpdateStateInMaintenanceRestoring,
	api.ServerUpdateStateInMaintenancePostRestore,
}

// clusterUpdateUpdateSteps is the number of leading entries of clusterUpdateSteps,
// which belong to the update phase, as opposed to the restart phase. A cluster
// update without reboot only passes through these states.
const clusterUpdateUpdateSteps = 2

// clusterUpdateProgress is the progress of an ongoing rolling cluster update.
// A step of 0 means, that there is nothing to report.
type clusterUpdateProgress struct {
	step       int
	totalSteps int
	state      api.ServerUpdateState
	serverName string

	// text, if set, is reported verbatim instead of the step based description.
	// It is used for the terminal error case.
	text string
}

func (p clusterUpdateProgress) String() string {
	if p.text != "" {
		return p.text
	}

	if p.step == 0 {
		return ""
	}

	format := fmt.Sprintf("[%%%[1]dd/%%%[1]dd] %%s server %%q", len(strconv.Itoa(p.totalSteps)))

	return fmt.Sprintf(format, p.step, p.totalSteps, p.state, p.serverName)
}

// serverPendingSteps returns the number of the perServerSteps steps, that the
// server in the given state still has ahead of it.
//
// States, which are not part of clusterUpdateSteps and which are not "up to date"
// are deliberately counted as not started at all. Under reporting the progress is
// harmless, since clusterUpdateProgressLatch keeps the last reported value in
// place, while over reporting would leave the reported progress ahead of reality
// for the rest of the run.
func serverPendingSteps(state api.ServerUpdateState, perServerSteps int) int {
	if state == api.ServerUpdateStateUpToDate {
		return 0
	}

	idx := slices.Index(clusterUpdateSteps, state)
	if idx < 0 {
		return perServerSteps
	}

	return min(max(perServerSteps-idx, 0), perServerSteps)
}

// serverUpdateStateForRollingUpdate returns the update state of the server as the
// rolling cluster update in the given phase sees it.
//
// During the rolling restart phase, pending updates are intentionally ignored. All
// servers of a cluster have been updated to the same version before entering the
// rolling restart, so a newly published update must not interrupt the ongoing
// restart cycle.
//
// During the update phase, a server updating its applications is reported as
// updating instead of the undefined state, api.Server.UpdateState reports for it,
// since applications are updated as part of the OS update.
func serverUpdateStateForRollingUpdate(inProgress api.ClusterUpdateInProgress, server provisioning.Server) api.ServerUpdateState {
	switch inProgress {
	case api.ClusterUpdateInProgressRollingRestart:
		server.VersionData.NeedsUpdate = ptr.To(false)

	case api.ClusterUpdateInProgressApplyUpdate,
		api.ClusterUpdateInProgressApplyUpdateWithReboot:
		if server.StatusDetail == api.ServerStatusDetailReadyUpdatingApplication {
			return api.ServerUpdateStateUpdating
		}
	}

	return server.UpdateState()
}

// clusterUpdateState calculates the progress of the rolling update of a cluster
// from the current state of its servers.
func clusterUpdateState(clusterUpdateInProgressStatus api.ClusterUpdateInProgressStatus, servers provisioning.Servers) clusterUpdateProgress {
	if clusterUpdateInProgressStatus.InProgress == api.ClusterUpdateInProgressError {
		return clusterUpdateProgress{
			text: clusterUpdateInProgressStatus.Error,
		}
	}

	var perServerSteps int
	switch clusterUpdateInProgressStatus.InProgress {
	case api.ClusterUpdateInProgressApplyUpdate:
		perServerSteps = clusterUpdateUpdateSteps

	case api.ClusterUpdateInProgressApplyUpdateWithReboot,
		api.ClusterUpdateInProgressRollingRestart:
		perServerSteps = len(clusterUpdateSteps)

	default:
		return clusterUpdateProgress{}
	}

	// Sort a copy, the caller's slice must not be reordered.
	servers = slices.Clone(servers)
	slices.SortStableFunc(servers, func(a provisioning.Server, b provisioning.Server) int {
		return strings.Compare(a.Name, b.Name)
	})

	// The server, the update is currently working on, is reported to the user. The
	// control loop updates all servers before it restarts any of them, so a server,
	// which is still being updated, takes precedence over a server, which is waiting
	// for its restart. A server in an unclassified state is only reported, if there
	// is nothing else left to report.
	var inUpdatePhase, inRestartPhase, unclassified *clusterUpdateProgress

	totalSteps := 0
	pendingSteps := 0

	for _, server := range servers {
		totalSteps += perServerSteps

		state := serverUpdateStateForRollingUpdate(clusterUpdateInProgressStatus.InProgress, server)

		// Servers, which have been evacuated before the update was triggered, are kept
		// in the evacuated state and therefore have no restore step ahead of them.
		if state == api.ServerUpdateStateInMaintenanceRestorePending &&
			slices.Contains(clusterUpdateInProgressStatus.EvacuatedBefore, server.Name) {
			continue
		}

		serverSteps := serverPendingSteps(state, perServerSteps)
		if serverSteps == 0 {
			continue
		}

		pendingSteps += serverSteps

		candidate := &clusterUpdateProgress{
			state:      state,
			serverName: server.Name,
		}

		idx := slices.Index(clusterUpdateSteps, state)

		switch {
		case idx < 0:
			if unclassified == nil {
				unclassified = candidate
			}

		case idx < clusterUpdateUpdateSteps:
			if inUpdatePhase == nil {
				inUpdatePhase = candidate
			}

		default:
			if inRestartPhase == nil {
				inRestartPhase = candidate
			}
		}
	}

	if pendingSteps == 0 {
		return clusterUpdateProgress{
			totalSteps: totalSteps,
		}
	}

	progress := firstProgress(inUpdatePhase, inRestartPhase, unclassified)
	progress.step = totalSteps - pendingSteps + 1
	progress.totalSteps = totalSteps

	return *progress
}

// firstProgress returns the first non nil progress candidate.
func firstProgress(candidates ...*clusterUpdateProgress) *clusterUpdateProgress {
	for _, candidate := range candidates {
		if candidate != nil {
			return candidate
		}
	}

	return &clusterUpdateProgress{}
}

// clusterUpdateProgressLatch keeps the highest progress observed per cluster, so
// that the progress reported to the user never moves backwards.
//
// The progress is recomputed from a live snapshot of the server states on every
// read, while the server states themselves are updated asynchronously from
// several sources (control loop, background polling, status updates pushed by
// IncusOS). A single server briefly falling back to an earlier state therefore
// makes the calculated progress go backwards, which is confusing for the user.
type clusterUpdateProgressLatch struct {
	mu       sync.Mutex
	progress map[string]clusterUpdateProgress
}

func newClusterUpdateProgressLatch() *clusterUpdateProgressLatch {
	return &clusterUpdateProgressLatch{
		progress: map[string]clusterUpdateProgress{},
	}
}

// apply returns the progress to report for the cluster, which is the highest
// progress observed for the current run of the rolling update so far.
func (l *clusterUpdateProgressLatch) apply(clusterName string, progress clusterUpdateProgress) clusterUpdateProgress {
	l.mu.Lock()
	defer l.mu.Unlock()

	// The terminal error is reported as is and ends the current run.
	if progress.text != "" {
		delete(l.progress, clusterName)
		return progress
	}

	// Nothing in progress, e.g. because the cluster has no servers.
	if progress.totalSteps == 0 {
		delete(l.progress, clusterName)
		return progress
	}

	previous, ok := l.progress[clusterName]

	// A different number of total steps means, this is not the same run anymore,
	// e.g. because a server has been added to or removed from the cluster.
	if !ok || previous.totalSteps != progress.totalSteps {
		l.progress[clusterName] = progress
		return progress
	}

	// All servers are done, but the update in progress status has not been advanced
	// to the next phase yet. Keep reporting the last known progress instead of
	// letting the description disappear for a moment.
	if progress.step == 0 {
		return previous
	}

	if progress.step < previous.step {
		return previous
	}

	l.progress[clusterName] = progress

	return progress
}

// reset drops the progress recorded for the cluster, so that the next rolling
// update of this cluster starts reporting from the beginning again.
func (l *clusterUpdateProgressLatch) reset(clusterName string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.progress, clusterName)
}
