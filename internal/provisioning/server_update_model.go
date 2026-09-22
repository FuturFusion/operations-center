package provisioning

import (
	"slices"
	"time"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

// ServerUpdateStep is the action of a cluster wide rolling update or rolling
// reboot, that Operations Center has triggered on a server.
type ServerUpdateStep string

const (
	// ServerUpdateStepNone means, that the server is not working on any step.
	ServerUpdateStepNone ServerUpdateStep = ""

	// ServerUpdateStepUpdate stages the IncusOS update and updates the
	// applications.
	ServerUpdateStepUpdate ServerUpdateStep = "update"

	// ServerUpdateStepEvacuate moves the instances of the server away, so it can
	// be rebooted.
	ServerUpdateStepEvacuate ServerUpdateStep = "evacuate"

	// ServerUpdateStepReboot reboots the server, which is what activates a staged
	// IncusOS update.
	ServerUpdateStepReboot ServerUpdateStep = "reboot"

	// ServerUpdateStepRestore moves the instances back to the server.
	ServerUpdateStepRestore ServerUpdateStep = "restore"
)

// serverUpdateStepDefinition describes a step of a cluster wide rolling update
// or rolling reboot.
type serverUpdateStepDefinition struct {
	// details are the status details Operations Center records, while it waits for
	// the outcome of the step.
	details []api.ServerStatusDetail

	// timeout is the time granted to a single attempt of the step, after which
	// nothing is going to report its outcome anymore. The clock starts, when the
	// step is triggered, so the trigger and the wait share it.
	timeout time.Duration

	// retries is the number of attempts the step is granted. The first trigger
	// spends one, so a step, which keeps failing, is given up after this many.
	retries int
}

// serverUpdateSteps holds what Operations Center records about every step it
// triggers on a server.
var serverUpdateSteps = map[ServerUpdateStep]serverUpdateStepDefinition{
	ServerUpdateStepUpdate: {
		// An update of the OS covers the applications as well, but the two report
		// their progress apart.
		details: []api.ServerStatusDetail{
			api.ServerStatusDetailReadyUpdatingOS,
			api.ServerStatusDetailReadyUpdatingApplication,
		},
		timeout: config.ClusterRollingUpdateApplyTimeout,
		retries: config.ClusterRollingUpdateStepRetries,
	},

	ServerUpdateStepEvacuate: {
		details: []api.ServerStatusDetail{api.ServerStatusDetailReadyEvacuating},
		timeout: config.ClusterRollingUpdateEvacuateTimeout,
		retries: config.ClusterRollingUpdateStepRetries,
	},

	ServerUpdateStepReboot: {
		details: []api.ServerStatusDetail{api.ServerStatusDetailOfflineRebooting},
		timeout: config.ClusterRollingUpdateRebootTimeout,
		retries: config.ClusterRollingUpdateStepRetries,
	},

	ServerUpdateStepRestore: {
		details: []api.ServerStatusDetail{api.ServerStatusDetailReadyRestoring},
		timeout: config.ClusterRollingUpdateRestoreTimeout,
		retries: config.ClusterRollingUpdateStepRetries,
	},
}

// Timeout is the time granted to the step, after which nothing is going to
// report its outcome anymore. A server, which is not working on any step, has
// none.
func (s ServerUpdateStep) Timeout() time.Duration {
	return serverUpdateSteps[s].timeout
}

// Retries is the number of attempts the step is granted. The first trigger
// spends one, so a step, which keeps failing, is given up after this many.
func (s ServerUpdateStep) Retries() int {
	return serverUpdateSteps[s].retries
}

// OwnsStatusDetail reports, whether the status detail is one Operations Center
// records while it waits for the outcome of the step.
func (s ServerUpdateStep) OwnsStatusDetail(statusDetail api.ServerStatusDetail) bool {
	return slices.Contains(serverUpdateSteps[s].details, statusDetail)
}

// ServerUpdate is what a cluster wide rolling update or rolling reboot has
// decided about a single server. It is persisted in Server.StatusInternal, so
// neither a daemon restart nor an operation, whose completion is never reported,
// leaves the run stuck.
//
// It records what Operations Center has asked of the server, while
// api.Server.UpdateState reports what the server is observed to be doing. The
// wait conditions are deliberately not persisted: they are re-derivable from
// freshly polled data, by the same derivation, that reports the progress, so the
// action taken and the progress shown can not disagree.
type ServerUpdate struct {
	// Step is the step, the server is working on, and StepTriggeredAt is when
	// the current attempt of it has been triggered. A zero StepTriggeredAt means,
	// that nothing is outstanding, either because the step has completed or
	// because the attempt has failed and the next one is due.
	Step            ServerUpdateStep `json:"step,omitempty"`
	StepTriggeredAt time.Time        `json:"step_triggered_at,omitzero"`

	// RebootPending records, that the server still owes the reboot, that
	// activates a staged IncusOS update.
	//
	// It is recorded when the update is triggered, rather than derived from
	// VersionData.NeedsReboot, because IncusOS reports the staged version and the
	// need for a reboot from two different endpoints, which do not flip together.
	RebootPending bool `json:"reboot_pending,omitempty"`

	// KeepEvacuated records, that the server was already evacuated when the run
	// was launched, so it is left evacuated afterwards.
	KeepEvacuated bool `json:"keep_evacuated,omitempty"`

	// Retries counts the attempts already spent on Step. It is kept across the
	// attempts of one step and reset when the server advances to the next.
	Retries int `json:"retries,omitempty"`

	// FirstError and LastError are the errors of the first and of the most recent
	// failed attempt of Step. The first one is kept as well, because it is
	// usually the one, which describes the actual problem: a failed attempt can
	// leave the server in a state, which makes every later attempt fail with a
	// different, derived error.
	FirstError string `json:"first_error,omitempty"`
	LastError  string `json:"last_error,omitempty"`

	// FailedAt is the time of the most recent failed attempt of Step, from which
	// the backoff before the next attempt is measured.
	FailedAt time.Time `json:"failed_at,omitzero"`

	StartedAt time.Time `json:"started_at,omitzero"`

	// Triggered holds the components of the update in flight, together with the
	// version each of them is expected to reach. It is recorded by every update,
	// whether it was requested for the single server or by a cluster wide run,
	// and is dropped once the update it describes has landed.
	Triggered *ServerTriggeredUpdate `json:"triggered,omitempty"`
}

// IsActive reports, whether the server takes part in a cluster wide run. A
// record, which only holds the triggered update of a standalone update, does
// not: StartedAt is stamped by BeginUpdateRunByCluster alone.
func (u *ServerUpdate) IsActive() bool {
	return u != nil && !u.StartedAt.IsZero()
}

// TriggeredUpdate returns the record of the update in flight, if there is one.
func (u *ServerUpdate) TriggeredUpdate() *ServerTriggeredUpdate {
	if u == nil {
		return nil
	}

	return u.Triggered
}

// ClearTriggered drops the record of the triggered update, keeping whatever a
// cluster wide run has recorded about the server.
func (u *ServerUpdate) ClearTriggered() {
	if u == nil {
		return
	}

	u.Triggered = nil
}

// IsEmpty reports, whether nothing is recorded about the server anymore, in
// which case the record itself can be dropped.
func (u *ServerUpdate) IsEmpty() bool {
	return u != nil && u.Triggered == nil && u.StartedAt.IsZero()
}

// EndRun drops what a cluster wide run has recorded about the server, keeping
// the triggered update, which outlives the run.
func (u *ServerUpdate) EndRun() {
	if u == nil {
		return
	}

	u.ReleaseAll()

	u.RebootPending = false
	u.KeepEvacuated = false
	u.StartedAt = time.Time{}
}

// KeepsEvacuated reports, whether the server was already evacuated when the run
// was launched, in which case it is left evacuated afterwards.
func (u *ServerUpdate) KeepsEvacuated() bool {
	return u != nil && u.KeepEvacuated
}

// RebootOwed reports, whether the server still has to be rebooted.
func (u *ServerUpdate) RebootOwed() bool {
	return u != nil && u.RebootPending
}

// InFlight reports, whether a step has been triggered and is still within the
// time it has been granted. Everything else is due to be triggered.
func (u *ServerUpdate) InFlight(now time.Time) bool {
	return u != nil && !u.StepTriggeredAt.IsZero() && now.Sub(u.StepTriggeredAt) <= u.Step.Timeout()
}

// InFlightStep returns the step, that has been triggered and is still within the
// time it has been granted, or none.
func (u *ServerUpdate) InFlightStep(now time.Time) ServerUpdateStep {
	if !u.InFlight(now) {
		return ServerUpdateStepNone
	}

	return u.Step
}

// Claim records, that the given step is about to be triggered. The retry budget
// is kept for another attempt of the same step and reset, once the server
// advances to the next one.
func (u *ServerUpdate) Claim(now time.Time, step ServerUpdateStep) {
	if u == nil {
		return
	}

	if u.Step != step {
		u.Step = step
		u.Retries = 0
		u.FirstError = ""
		u.LastError = ""
		u.FailedAt = time.Time{}
	}

	u.StepTriggeredAt = now
	u.Retries++
}

// Release records, that the given step is done. It is a no-op for a step, that
// is not the one the server is working on, so an outcome observed for an earlier
// step can not retire the current one.
func (u *ServerUpdate) Release(step ServerUpdateStep) {
	if u == nil || u.Step != step {
		return
	}

	u.ReleaseAll()
}

// ReleaseAll drops everything recorded about the step in flight, keeping what
// the run has decided about the server.
func (u *ServerUpdate) ReleaseAll() {
	if u == nil {
		return
	}

	u.Step = ServerUpdateStepNone
	u.StepTriggeredAt = time.Time{}
	u.Retries = 0
	u.FirstError = ""
	u.LastError = ""
	u.FailedAt = time.Time{}
}

// Fail records, that the attempt of the given step has failed. The step and the
// attempts spent on it are kept, so the next attempt is counted against the same
// budget.
func (u *ServerUpdate) Fail(now time.Time, step ServerUpdateStep, err error) {
	if u == nil || u.Step != step {
		return
	}

	u.StepTriggeredAt = time.Time{}
	u.FailedAt = now
	u.LastError = domain.UserMessage(err)

	if u.FirstError == "" {
		u.FirstError = u.LastError
	}
}

// RetryBackoffRemaining returns the time, which still has to pass after the most
// recent failed attempt of the step, before the next one may be triggered. A
// step, which has not failed yet, backs off for nothing.
func (u *ServerUpdate) RetryBackoffRemaining(now time.Time, backoff time.Duration) time.Duration {
	if u == nil || u.FailedAt.IsZero() {
		return 0
	}

	return max(u.FailedAt.Add(backoff).Sub(now), 0)
}

// ServerTriggeredUpdate records the components an update has been triggered for
// together with the version each of them is expected to reach.
type ServerTriggeredUpdate struct {
	// OS is the version the OS is expected to reach.
	OS string `json:"os,omitempty"`

	// Applications maps the name of every application an update has been triggered for.
	Applications map[string]string `json:"applications,omitempty"`

	// TriggeredAt is the point in time the update has been triggered.
	TriggeredAt time.Time `json:"triggered_at"`
}

// IsPending reports whether any of the triggered components still has work ahead of it.
func (t *ServerTriggeredUpdate) IsPending(versionData api.ServerVersionData) bool {
	if t == nil {
		return false
	}

	if t.OS != "" && ptr.From(versionData.OS.NeedsUpdate) &&
		versionData.OS.Version != t.OS && versionData.OS.VersionNext != t.OS {
		return true
	}

	for _, application := range versionData.Applications {
		expectedVersion, ok := t.Applications[application.Name]
		if !ok {
			continue
		}

		if ptr.From(application.NeedsUpdate) && application.Version != expectedVersion {
			return true
		}
	}

	return false
}
