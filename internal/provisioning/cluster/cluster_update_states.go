package cluster

import (
	"context"
	"time"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// rollingUpdateStateKind tells the control loop, how the server it is working on
// is moved out of its current update state.
type rollingUpdateStateKind int

const (
	// rollingUpdateStateKindUnsupported is a state, the run has no way to drive a
	// server out of. It is the zero value, so a state, which is not described in
	// rollingUpdateStates, ends the run instead of being silently skipped.
	rollingUpdateStateKindUnsupported rollingUpdateStateKind = iota

	// rollingUpdateStateKindDone is a state, in which the server has no work left.
	rollingUpdateStateKindDone

	// rollingUpdateStateKindTrigger issues the step of the state.
	rollingUpdateStateKindTrigger

	// rollingUpdateStateKindWait waits for the step, which has been triggered.
	rollingUpdateStateKindWait

	// rollingUpdateStateKindSettle waits for a delay to pass, which no step is
	// accounted against.
	rollingUpdateStateKindSettle
)

// rollingUpdateOutOfOrder tells the control loop, how a server in a given update
// state is treated, while the run works on another one.
type rollingUpdateOutOfOrder int

const (
	// rollingUpdateOutOfOrderBenign is a state, a server may sit in until its turn
	// comes.
	rollingUpdateOutOfOrderBenign rollingUpdateOutOfOrder = iota

	// rollingUpdateOutOfOrderBlocks is a state, only the server, the run is
	// working on, may be in.
	rollingUpdateOutOfOrderBlocks
)

// rollingUpdateKeptEvacuated tells the control loop, how a server, which the run
// leaves evacuated, is treated in a state, which would otherwise put it back
// into service.
type rollingUpdateKeptEvacuated int

const (
	rollingUpdateKeptEvacuatedNone rollingUpdateKeptEvacuated = iota

	// rollingUpdateKeptEvacuatedTolerated only exempts the server from blocking
	// the one, the run is working on. It still owes the step of the state, since
	// being left evacuated does not spare it the reboot, which activates its
	// staged update.
	rollingUpdateKeptEvacuatedTolerated

	// rollingUpdateKeptEvacuatedDone additionally leaves the server here, which is
	// where the reported progress stops counting steps for it as well.
	rollingUpdateKeptEvacuatedDone
)

// rollingUpdateStateDefinition describes, how the control loop treats a server in
// a single update state.
type rollingUpdateStateDefinition struct {
	kind rollingUpdateStateKind

	// step is the step a trigger issues and a wait waits for.
	step provisioning.ServerUpdateStep

	// retries is the number of attempts a trigger is granted for its step, which
	// it shares with the wait, that falls back to it.
	retries int

	// timeout is the time a wait is granted, after which nothing is going to
	// report the outcome of the step anymore.
	timeout time.Duration

	// settleDelay is the time a settle waits for, before it moves the server on.
	// In contrast to a timeout, it is a minimum rather than a maximum.
	settleDelay time.Duration

	// retrigger says, whether a wait, whose step has stalled, falls back to
	// issuing the step again. A step, which can not be issued a second time, ends
	// the run instead.
	retrigger bool

	keptEvacuated rollingUpdateKeptEvacuated
	outOfOrder    rollingUpdateOutOfOrder
}

// rollingUpdateStates holds the per server state machine of a cluster wide
// rolling restart or rolling reboot.
//
// api.ServerUpdateStateUndefined and api.ServerUpdateStateUpdating are
// deliberately absent: neither is a state of this machine, both end the run
// wherever they are observed.
var rollingUpdateStates = map[api.ServerUpdateState]rollingUpdateStateDefinition{
	api.ServerUpdateStateUpToDate: {
		kind: rollingUpdateStateKindDone,
	},

	// serverUpdateStateForRollingUpdate reports NeedsUpdate = false, so the run
	// never observes a pending update.
	api.ServerUpdateStateUpdatePending: {
		kind: rollingUpdateStateKindUnsupported,
	},

	api.ServerUpdateStateEvacuationPending: {
		kind:    rollingUpdateStateKindTrigger,
		step:    provisioning.ServerUpdateStepEvacuate,
		retries: config.ClusterRollingUpdateStepRetries,
	},

	api.ServerUpdateStateEvacuating: {
		kind:       rollingUpdateStateKindWait,
		step:       provisioning.ServerUpdateStepEvacuate,
		timeout:    config.ClusterRollingUpdateEvacuateTimeout,
		retrigger:  true,
		outOfOrder: rollingUpdateOutOfOrderBlocks,
	},

	api.ServerUpdateStateInMaintenanceRebootPending: {
		kind:          rollingUpdateStateKindTrigger,
		step:          provisioning.ServerUpdateStepReboot,
		retries:       config.ClusterRollingUpdateStepRetries,
		keptEvacuated: rollingUpdateKeptEvacuatedTolerated,
		outOfOrder:    rollingUpdateOutOfOrderBlocks,
	},

	api.ServerUpdateStateInMaintenanceRebooting: {
		// A server, which does not come back, can not be rebooted again, so the step
		// has no trigger to fall back to and ends the run instead.
		kind:       rollingUpdateStateKindWait,
		step:       provisioning.ServerUpdateStepReboot,
		timeout:    config.ClusterRollingUpdateRebootTimeout,
		outOfOrder: rollingUpdateOutOfOrderBlocks,
	},

	api.ServerUpdateStateInMaintenanceRestorePending: {
		kind:          rollingUpdateStateKindTrigger,
		step:          provisioning.ServerUpdateStepRestore,
		retries:       config.ClusterRollingUpdateStepRetries,
		keptEvacuated: rollingUpdateKeptEvacuatedDone,
		outOfOrder:    rollingUpdateOutOfOrderBlocks,
	},

	api.ServerUpdateStateInMaintenanceRestoring: {
		kind:       rollingUpdateStateKindWait,
		step:       provisioning.ServerUpdateStepRestore,
		timeout:    config.ClusterRollingUpdateRestoreTimeout,
		retrigger:  true,
		outOfOrder: rollingUpdateOutOfOrderBlocks,
	},

	api.ServerUpdateStateInMaintenancePostRestore: {
		// A cluster, which configures no post restore delay, is granted this one,
		// rather than no delay at all.
		kind:        rollingUpdateStateKindSettle,
		settleDelay: config.ClusterRollingUpdatePostRestoreDelay,
		outOfOrder:  rollingUpdateOutOfOrderBlocks,
	},

	// A server, which is not a member of a clustered Incus, can not be evacuated,
	// so the run has no way to drive it through the cycle.
	api.ServerUpdateStateRebootPending: {
		kind:       rollingUpdateStateKindUnsupported,
		outOfOrder: rollingUpdateOutOfOrderBlocks,
	},

	api.ServerUpdateStateRebooting: {
		kind:       rollingUpdateStateKindUnsupported,
		outOfOrder: rollingUpdateOutOfOrderBlocks,
	},
}

// rollingUpdateStepTrigger returns the call, which issues the given step on the
// server. It is shared by the state, which triggers the step, and by the wait,
// which falls back to issuing it again.
func (s *clusterService) rollingUpdateStepTrigger(cluster provisioning.Cluster, server provisioning.Server, step provisioning.ServerUpdateStep) func(context.Context) error {
	switch step {
	case provisioning.ServerUpdateStepUpdate:
		// An update of the OS covers the applications as well, so the whole server
		// is brought up to date with a single trigger.
		return func(ctx context.Context) error {
			return s.serverSvc.UpdateSystemByName(ctx, server.Name, api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
			}, true)
		}

	case provisioning.ServerUpdateStepEvacuate:
		return func(ctx context.Context) error {
			return s.serverSvc.EvacuateSystemByName(ctx, server.Name, true, false)
		}

	case provisioning.ServerUpdateStepReboot:
		return func(ctx context.Context) error {
			return s.serverSvc.RebootSystemByName(ctx, server.Name, true)
		}

	case provisioning.ServerUpdateStepRestore:
		restoreModeSkip := cluster.Config.RollingRestart.RestoreMode == "skip"

		return func(ctx context.Context) error {
			return s.serverSvc.RestoreSystemByName(ctx, server.Name, true, false, restoreModeSkip)
		}
	}

	return nil
}

// rollingRestartPostRestoreDelay is the time waited between the restore of a
// server and the evacuation of the next one, which grants the cluster enough
// time to move the instances back to the restored server.
func rollingRestartPostRestoreDelay(cluster provisioning.Cluster, defaultDelay time.Duration) time.Duration {
	if cluster.Config.RollingRestart.PostRestoreDelay == "" {
		return defaultDelay
	}

	// The duration is validated on save, so it can only fail to parse if it is
	// empty, which is handled above.
	postRestoreDelay, _ := time.ParseDuration(cluster.Config.RollingRestart.PostRestoreDelay)

	return postRestoreDelay
}

// rollingUpdateSettle ends the settle period after the restore of a server, or
// parks the run until the delay has passed. Nothing has been triggered on the
// server, so there is no step to account the period against and no attempt to
// spend on it.
func (s *clusterService) rollingUpdateSettle(cluster provisioning.Cluster, server provisioning.Server) func(context.Context) error {
	// The default is taken from the state, this is the settle of, so there is no
	// way to drive the settle with a delay, the table does not declare.
	defaultDelay := rollingUpdateStates[api.ServerUpdateStateInMaintenancePostRestore].settleDelay

	if server.LastStatusUpdated.Add(rollingRestartPostRestoreDelay(cluster, defaultDelay)).Before(s.now()) {
		return func(ctx context.Context) error {
			return s.serverSvc.PostRestoreSystemDoneByName(ctx, server.Name)
		}
	}

	return func(ctx context.Context) error {
		return nil
	}
}
