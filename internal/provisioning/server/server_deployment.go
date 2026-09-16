package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/stmcginnis/gofish/schemas"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

// deploymentStateKind tells the dispatcher, how a state of the deployment state
// machine is driven forward.
type deploymentStateKind int

const (
	deploymentStateKindAction deploymentStateKind = iota
	deploymentStateKindWait
	deploymentStateKindTerminal
)

// deploymentWaitFunc evaluates the condition of a wait state.
type deploymentWaitFunc func(*serverService, context.Context, *slog.Logger, provisioning.Server, deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error)

// deploymentBIOSPass tells, which of the two BIOS passes a state belongs to,
// which the attributes to apply and to verify are picked by.
type deploymentBIOSPass int

const (
	deploymentBIOSPassNone deploymentBIOSPass = iota
	deploymentBIOSPassFirst
	deploymentBIOSPassDeferred
)

// deploymentInstallThresholds holds, what the install wait tells the first stage
// of the installation by.
type deploymentInstallThresholds struct {
	// minDuration is the time, that has to have passed, before the installation
	// could be done at all.
	minDuration time.Duration

	// rebootFallbackDelay is the time, after which a reboot counts as the end of
	// the first stage, where nothing better is available.
	rebootFallbackDelay time.Duration

	// mediaIdlePeriod is the time without a read, after which the installation
	// media counts as idle.
	mediaIdlePeriod time.Duration

	// mediaMinBytesRead is, how much of the installation media has to have been
	// read, before the idle period is taken as a signal.
	mediaMinBytesRead int64
}

// deploymentStateDefinition describes a single state of the deployment state
// machine. Every step is split into a trigger and a wait state, so a retry
// re-issues the trigger, a wait timeout falls back to it and a daemon restart
// re-enters the persisted state.
type deploymentStateDefinition struct {
	kind   deploymentStateKind
	detail api.ServerStatusDetail
	next   api.ServerDeploymentState

	// wait evaluates the condition of a wait state.
	wait deploymentWaitFunc

	// enterState returns the state the deployment enters in place of this one:
	// the state itself, or the state to skip to, where it has nothing left to do
	// here. Every skip moves forward, so the chain of them settles.
	enterState func(*provisioning.ServerDeployment) api.ServerDeploymentState

	// fallback is the trigger state a wait state returns to, when it times out.
	// An empty fallback fails the deployment instead.
	fallback api.ServerDeploymentState

	// retryFrom is the earlier state, a failed step routes the deployment back
	// to, instead of being retried in place.
	retryFrom api.ServerDeploymentState

	// retries is the number of attempts granted to the state, before the
	// deployment fails.
	retries int

	// settleDelay is the time, the server is granted to come up and run through
	// its power on self test, before a wait reads anything into what the BMC
	// reports. In contrast to a timeout, it is a minimum rather than a maximum.
	settleDelay time.Duration

	// rebootWindow is the time, a wait looks for an actual reboot of the server,
	// before it settles for the weaker signal it has.
	rebootWindow time.Duration

	// install holds, what the install wait tells the installation by.
	install deploymentInstallThresholds

	// biosPass names the BIOS pass the state belongs to.
	biosPass deploymentBIOSPass

	// status is the server status a terminal state reports. An empty status
	// leaves the server deploying.
	status api.ServerStatus

	// timeout bounds a wait state.
	timeout time.Duration

	// callTimeout bounds the BMC operations of a single attempt of the state,
	// defaulting to config.ServerDeploymentStepCallTimeout. In contrast to
	// timeout, which bounds how long a wait may stay unsatisfied, it keeps an
	// unresponsive BMC from parking the control loop.
	callTimeout time.Duration

	// cancelPhase marks the states, that clean a cancelled deployment up, and
	// which a cancellation therefore does not preempt.
	cancelPhase bool

	// prepare records, that the action of the state is about to run, and is
	// persisted before it does. It belongs to a step, whose side effect can not
	// be told apart from a no-op once it has been performed, so a re-issued
	// attempt has to learn from the record instead of from the BMC.
	prepare func(*provisioning.ServerDeployment)
}

func (d deploymentStateDefinition) callTimeoutOrDefault() time.Duration {
	if d.callTimeout > 0 {
		return d.callTimeout
	}

	return config.ServerDeploymentStepCallTimeout
}

// deploymentWaitPowerIsOff is shared by every state, that waits for the server
// to be powered off.
var deploymentWaitPowerIsOff = deploymentBMCWait(deploymentPowerIsOff)

// deploymentStates holds the deployment state machine. Every wait condition
// listed here is re-derivable from the BMC data or the server record alone, a
// persisted task monitor is only ever an optimization, since a BMC forgets
// about one once it has been consumed or has been reset.
var deploymentStates = map[api.ServerDeploymentState]deploymentStateDefinition{
	api.ServerDeploymentStateRefreshBMCData: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingPreparing,
		next:    api.ServerDeploymentStateCheckBIOS,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateCheckBIOS: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingPreparing,
		next:    api.ServerDeploymentStatePowerOffBIOS,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStatePowerOffBIOS: {
		kind:     deploymentStateKindAction,
		detail:   api.ServerStatusDetailDeployingPreparing,
		next:     api.ServerDeploymentStateWaitPowerOffBIOS,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassFirst,

		enterState: func(deployment *provisioning.ServerDeployment) api.ServerDeploymentState {
			if deployment.BIOSPending {
				return api.ServerDeploymentStatePowerOffBIOS
			}

			return api.ServerDeploymentStatePowerOffBIOSDeferred
		},
	},
	api.ServerDeploymentStateWaitPowerOffBIOS: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingPreparing,
		wait:     deploymentWaitPowerIsOff,
		next:     api.ServerDeploymentStateApplyBIOS,
		fallback: api.ServerDeploymentStatePowerOffBIOS,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassFirst,
	},
	api.ServerDeploymentStateApplyBIOS: {
		kind:     deploymentStateKindAction,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		next:     api.ServerDeploymentStatePowerOnBIOS,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassFirst,
	},
	api.ServerDeploymentStatePowerOnBIOS: {
		kind:     deploymentStateKindAction,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		next:     api.ServerDeploymentStateWaitBIOSApplied,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassFirst,
	},
	api.ServerDeploymentStateWaitBIOSApplied: {
		kind:        deploymentStateKindWait,
		detail:      api.ServerStatusDetailDeployingConfiguringBIOS,
		wait:        (*serverService).checkDeploymentBIOSApplied,
		next:        api.ServerDeploymentStateVerifyBIOS,
		fallback:    api.ServerDeploymentStatePowerOffBIOS,
		timeout:     config.ServerDeploymentStepWaitBIOSAppliedTimeout,
		retries:     config.ServerDeploymentStepRetries,
		settleDelay: config.ServerDeploymentSettleDelay,
		biosPass:    deploymentBIOSPassFirst,
	},
	api.ServerDeploymentStateVerifyBIOS: {
		kind:      deploymentStateKindAction,
		detail:    api.ServerStatusDetailDeployingConfiguringBIOS,
		next:      api.ServerDeploymentStatePowerOffBIOSDeferred,
		retries:   config.ServerDeploymentStepRetries,
		retryFrom: api.ServerDeploymentStatePowerOffBIOS,
		biosPass:  deploymentBIOSPassFirst,
	},
	api.ServerDeploymentStatePowerOffBIOSDeferred: {
		kind:     deploymentStateKindAction,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		next:     api.ServerDeploymentStateWaitPowerOffBIOSDeferred,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassDeferred,

		enterState: func(deployment *provisioning.ServerDeployment) api.ServerDeploymentState {
			if deployment.BIOSDeferredPending {
				return api.ServerDeploymentStatePowerOffBIOSDeferred
			}

			return api.ServerDeploymentStatePowerOffSecureBoot
		},
	},
	api.ServerDeploymentStateWaitPowerOffBIOSDeferred: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		wait:     deploymentWaitPowerIsOff,
		next:     api.ServerDeploymentStateApplyBIOSDeferred,
		fallback: api.ServerDeploymentStatePowerOffBIOSDeferred,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassDeferred,
	},
	api.ServerDeploymentStateApplyBIOSDeferred: {
		kind:     deploymentStateKindAction,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		next:     api.ServerDeploymentStatePowerOnBIOSDeferred,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassDeferred,
	},
	api.ServerDeploymentStatePowerOnBIOSDeferred: {
		kind:     deploymentStateKindAction,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		next:     api.ServerDeploymentStateWaitBIOSAppliedDeferred,
		retries:  config.ServerDeploymentStepRetries,
		biosPass: deploymentBIOSPassDeferred,
	},
	api.ServerDeploymentStateWaitBIOSAppliedDeferred: {
		kind:        deploymentStateKindWait,
		detail:      api.ServerStatusDetailDeployingConfiguringBIOS,
		wait:        (*serverService).checkDeploymentBIOSApplied,
		next:        api.ServerDeploymentStateVerifyBIOSDeferred,
		fallback:    api.ServerDeploymentStatePowerOffBIOSDeferred,
		timeout:     config.ServerDeploymentStepWaitBIOSAppliedTimeout,
		retries:     config.ServerDeploymentStepRetries,
		settleDelay: config.ServerDeploymentSettleDelay,
		biosPass:    deploymentBIOSPassDeferred,
	},
	api.ServerDeploymentStateVerifyBIOSDeferred: {
		kind:      deploymentStateKindAction,
		detail:    api.ServerStatusDetailDeployingConfiguringBIOS,
		next:      api.ServerDeploymentStatePowerOffSecureBoot,
		retries:   config.ServerDeploymentStepRetries,
		retryFrom: api.ServerDeploymentStatePowerOffBIOSDeferred,
		biosPass:  deploymentBIOSPassDeferred,
	},
	api.ServerDeploymentStatePowerOffSecureBoot: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingConfiguringBIOS,
		next:    api.ServerDeploymentStateWaitPowerOffSecureBoot,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateWaitPowerOffSecureBoot: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		wait:     deploymentWaitPowerIsOff,
		next:     api.ServerDeploymentStateSecureBoot,
		fallback: api.ServerDeploymentStatePowerOffSecureBoot,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateSecureBoot: {
		kind:        deploymentStateKindAction,
		detail:      api.ServerStatusDetailDeployingConfiguringBIOS,
		next:        api.ServerDeploymentStateClearMedia,
		retries:     config.ServerDeploymentStepRetries,
		callTimeout: config.ServerDeploymentSecureBootCallTimeout,

		enterState: func(deployment *provisioning.ServerDeployment) api.ServerDeploymentState {
			if !deployment.Request.SkipSecureBootCertificates {
				return api.ServerDeploymentStateSecureBoot
			}

			return api.ServerDeploymentStateClearMedia
		},
		prepare: func(deployment *provisioning.ServerDeployment) {
			deployment.SecureBootAttempted = true
		},
	},
	api.ServerDeploymentStateClearMedia: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingAttachingMedia,
		next:    api.ServerDeploymentStateWaitMediaCleared,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateWaitMediaCleared: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingAttachingMedia,
		wait:     deploymentBMCWait(deploymentNoMediaInserted),
		next:     api.ServerDeploymentStatePowerOnSecureBoot,
		fallback: api.ServerDeploymentStateClearMedia,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStatePowerOnSecureBoot: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingConfiguringBIOS,
		next:    api.ServerDeploymentStateWaitSecureBootSettled,
		retries: config.ServerDeploymentStepRetries,

		enterState: func(deployment *provisioning.ServerDeployment) api.ServerDeploymentState {
			if deployment.SecureBootPending {
				return api.ServerDeploymentStatePowerOnSecureBoot
			}

			return api.ServerDeploymentStateAttachMedia
		},
	},
	api.ServerDeploymentStateWaitSecureBootSettled: {
		kind:         deploymentStateKindWait,
		detail:       api.ServerStatusDetailDeployingConfiguringBIOS,
		wait:         (*serverService).checkDeploymentSecureBootSettled,
		next:         api.ServerDeploymentStatePowerOffSecureBootSettled,
		fallback:     api.ServerDeploymentStatePowerOnSecureBoot,
		timeout:      config.ServerDeploymentStepWaitBIOSAppliedTimeout,
		retries:      config.ServerDeploymentStepRetries,
		settleDelay:  config.ServerDeploymentSettleDelay,
		rebootWindow: config.ServerDeploymentSecureBootSettleDuration,
	},
	api.ServerDeploymentStatePowerOffSecureBootSettled: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingConfiguringBIOS,
		next:    api.ServerDeploymentStateWaitPowerOffSecureBootSettled,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateWaitPowerOffSecureBootSettled: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingConfiguringBIOS,
		wait:     deploymentWaitPowerIsOff,
		next:     api.ServerDeploymentStateAttachMedia,
		fallback: api.ServerDeploymentStatePowerOffSecureBootSettled,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateAttachMedia: {
		kind:        deploymentStateKindAction,
		detail:      api.ServerStatusDetailDeployingAttachingMedia,
		next:        api.ServerDeploymentStateWaitMediaAttached,
		retries:     config.ServerDeploymentStepRetries,
		callTimeout: config.ServerDeploymentAttachMediaCallTimeout,
	},
	api.ServerDeploymentStateWaitMediaAttached: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingAttachingMedia,
		wait:     (*serverService).checkDeploymentMediaAttached,
		next:     api.ServerDeploymentStatePowerOnInstall,
		fallback: api.ServerDeploymentStateAttachMedia,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStatePowerOnInstall: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingInstalling,
		next:    api.ServerDeploymentStateWaitInstall,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateWaitInstall: {
		kind:        deploymentStateKindWait,
		detail:      api.ServerStatusDetailDeployingInstalling,
		wait:        (*serverService).checkDeploymentInstalled,
		next:        api.ServerDeploymentStateDetachMedia,
		timeout:     config.ServerDeploymentInstallTimeout,
		settleDelay: config.ServerDeploymentSettleDelay,
		install: deploymentInstallThresholds{
			minDuration:         config.ServerDeploymentMinInstallDuration,
			rebootFallbackDelay: config.ServerDeploymentInstallRebootFallbackDelay,
			mediaIdlePeriod:     config.ServerDeploymentMediaIdlePeriod,
			mediaMinBytesRead:   config.ServerDeploymentMediaMinBytesRead,
		},
	},
	api.ServerDeploymentStateDetachMedia: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingFinalizing,
		next:    api.ServerDeploymentStateWaitMediaDetached,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateWaitMediaDetached: {
		kind:     deploymentStateKindWait,
		detail:   api.ServerStatusDetailDeployingFinalizing,
		wait:     deploymentBMCWait(deploymentMediaEjected),
		next:     api.ServerDeploymentStateWaitReboot,
		fallback: api.ServerDeploymentStateDetachMedia,
		timeout:  config.ServerDeploymentStepTimeout,
		retries:  config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateWaitReboot: {
		kind:         deploymentStateKindWait,
		detail:       api.ServerStatusDetailDeployingFinalizing,
		wait:         (*serverService).checkDeploymentRebooted,
		next:         api.ServerDeploymentStateWaitRegistration,
		timeout:      config.ServerDeploymentRebootTimeout,
		rebootWindow: config.ServerDeploymentRebootObservationWindow,
	},
	api.ServerDeploymentStateWaitRegistration: {
		kind:    deploymentStateKindWait,
		detail:  api.ServerStatusDetailDeployingFinalizing,
		wait:    (*serverService).checkDeploymentRegistered,
		next:    api.ServerDeploymentStateCleanup,
		timeout: config.ServerDeploymentRegistrationTimeout,
	},
	api.ServerDeploymentStateCleanup: {
		kind:    deploymentStateKindAction,
		detail:  api.ServerStatusDetailDeployingFinalizing,
		next:    api.ServerDeploymentStateCompleted,
		retries: config.ServerDeploymentStepRetries,
	},
	api.ServerDeploymentStateCancel: {
		kind:        deploymentStateKindAction,
		detail:      api.ServerStatusDetailDeployingCancelling,
		next:        api.ServerDeploymentStateWaitCancel,
		retries:     config.ServerDeploymentStepRetries,
		cancelPhase: true,
	},
	api.ServerDeploymentStateWaitCancel: {
		kind:        deploymentStateKindWait,
		detail:      api.ServerStatusDetailDeployingCancelling,
		wait:        deploymentBMCWait(deploymentCancelSettled),
		next:        api.ServerDeploymentStateCancelled,
		fallback:    api.ServerDeploymentStateCancel,
		timeout:     config.ServerDeploymentStepTimeout,
		retries:     config.ServerDeploymentStepRetries,
		cancelPhase: true,
	},
	// A registration would have moved the server out of status deploying already,
	// so reaching the completed state means the deployment completed without one.
	api.ServerDeploymentStateCompleted: {
		kind:   deploymentStateKindTerminal,
		status: api.ServerStatusUnregistered,
	},
	api.ServerDeploymentStateFailed: {
		kind:   deploymentStateKindTerminal,
		status: api.ServerStatusUnregistered,
		detail: api.ServerStatusDetailUnregisteredDeploymentFailed,
	},
	api.ServerDeploymentStateCancelled: {
		kind:   deploymentStateKindTerminal,
		status: api.ServerStatusUnregistered,
		detail: api.ServerStatusDetailUnregisteredDeploymentCancelled,
	},
}

// deploymentRetryFromError routes a failed step back to an earlier state of the
// deployment instead of retrying the step itself.
type deploymentRetryFromError struct {
	state api.ServerDeploymentState
	err   error
}

func (e deploymentRetryFromError) Error() string {
	return e.err.Error()
}

func (e deploymentRetryFromError) Unwrap() error {
	return e.err
}

// deploymentFatalError ends the deployment right away, instead of being retried.
// It is what a wait state reports, when it observes, that the condition it is
// waiting for can never be met.
type deploymentFatalError struct {
	err error
}

func (e deploymentFatalError) Error() string {
	return e.err.Error()
}

func (e deploymentFatalError) Unwrap() error {
	return e.err
}

// deploymentNextState returns the state, the deployment enters after the step it
// just completed. A state, that has nothing to do for this deployment, names the
// state to skip to instead of itself. The skips chain, but every one of them
// moves forward, so the loop settles.
func deploymentNextState(deployment *provisioning.ServerDeployment, next api.ServerDeploymentState) api.ServerDeploymentState {
	// The table is finite and every skip moves forward, so this many rounds can
	// not be reached. The bound keeps a table, that does not, from parking the
	// control loop.
	for range len(deploymentStates) {
		enterState := deploymentStates[next].enterState
		if enterState == nil {
			return next
		}

		entered := enterState(deployment)
		if entered == next {
			return next
		}

		next = entered
	}

	return next
}

// deploymentBackoff returns how long to wait before the n-th attempt of a step.
func deploymentBackoff(retries int) time.Duration {
	if retries < 1 {
		return 0
	}

	backoff := config.ServerDeploymentRetryBackoff << min(retries-1, 10)
	if backoff > config.ServerDeploymentRetryBackoffMax || backoff <= 0 {
		return config.ServerDeploymentRetryBackoffMax
	}

	return backoff
}

// deploymentRun keeps what the timer driven and the event driven entries into
// the control loop share for a single server.
type deploymentRun struct {
	// mu is held for as long as the deployment of the server is advanced, so the
	// entries do not interleave.
	mu sync.Mutex

	// state guards the fields below, which are read while mu is held by somebody
	// else.
	state sync.Mutex

	// triggered records, that an entry found the deployment busy. The holder
	// picks the trigger up instead of dropping it, so an event, that satisfies a
	// wait, does not have to be waited out by the next tick.
	triggered bool

	// cancelStep interrupts the step in flight, if there is one. It belongs to a
	// single attempt, so the step, that follows a cancellation, starts from a
	// context of its own.
	cancelStep context.CancelFunc

	// users counts, how many callers hold the run. It is guarded by the registry
	// of the service, not by state.
	users int
}

// takeTrigger reports, whether an entry has been dropped while the deployment
// was advanced, and clears the record of it.
func (r *deploymentRun) takeTrigger() bool {
	r.state.Lock()
	defer r.state.Unlock()

	triggered := r.triggered
	r.triggered = false

	return triggered
}

func (r *deploymentRun) recordTrigger() {
	r.state.Lock()
	defer r.state.Unlock()

	r.triggered = true
}

func (r *deploymentRun) setCancelStep(cancel context.CancelFunc) {
	r.state.Lock()
	defer r.state.Unlock()

	r.cancelStep = cancel
}

// cancel interrupts the step in flight, if there is one.
func (r *deploymentRun) cancel() {
	r.state.Lock()
	cancelStep := r.cancelStep
	r.state.Unlock()

	if cancelStep != nil {
		cancelStep()
	}
}

// acquireDeploymentRun returns the state shared for the deployment of a server
// and registers the caller as a user of it. Every caller has to release it
// again.
func (s *serverService) acquireDeploymentRun(name string) *deploymentRun {
	s.deploymentControlLoopMu.Lock()
	defer s.deploymentControlLoopMu.Unlock()

	run, ok := s.deploymentControlLoopRuns[name]
	if !ok {
		run = &deploymentRun{}
		s.deploymentControlLoopRuns[name] = run
	}

	run.users++

	return run
}

// releaseDeploymentRun drops the state shared for the deployment of a server, as
// soon as nobody holds it anymore, so the registry does not keep an entry for
// every server ever deployed.
func (s *serverService) releaseDeploymentRun(name string, run *deploymentRun) {
	s.deploymentControlLoopMu.Lock()
	defer s.deploymentControlLoopMu.Unlock()

	run.users--

	if run.users == 0 && s.deploymentControlLoopRuns[name] == run {
		delete(s.deploymentControlLoopRuns, name)
	}
}

// lookupDeploymentRun returns the state shared for the deployment of a server,
// or nil, if the deployment is not being advanced right now.
func (s *serverService) lookupDeploymentRun(name string) *deploymentRun {
	s.deploymentControlLoopMu.Lock()
	defer s.deploymentControlLoopMu.Unlock()

	return s.deploymentControlLoopRuns[name]
}

// deploymentStepContext bounds a single attempt of a step and publishes its
// cancellation, so a cancelled deployment does not have to wait the attempt out.
// The caller holds the run of the server, so it can not vanish meanwhile.
func (s *serverService) deploymentStepContext(ctx context.Context, name string, timeout time.Duration) (context.Context, context.CancelFunc) {
	run := s.lookupDeploymentRun(name)

	ctx, cancel := context.WithTimeout(ctx, timeout)

	if run == nil {
		return ctx, cancel
	}

	run.setCancelStep(cancel)

	return ctx, func() {
		run.setCancelStep(nil)
		cancel()
	}
}

// DeployByName triggers the automated deployment of IncusOS on a server.
func (s *serverService) DeployByName(ctx context.Context, name string, request provisioning.ServerDeploymentRequest) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := request.Validate()
	if err != nil {
		return err
	}

	if config.GetNetwork().OperationsCenterAddress == "" {
		return fmt.Errorf("Operations Center address is not configured, the BMC would not be able to fetch the installation media: %w", domain.ErrOperationNotPermitted)
	}

	// Collect the BMC data before anything is checked against it.
	server, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	if !server.BMCConfig.HasBMC() {
		return fmt.Errorf("Server %q has no BMC configured: %w", name, domain.ErrOperationNotPermitted)
	}

	err = s.resyncBMCData(ctx, *server)
	if err != nil {
		return fmt.Errorf("Failed to collect the BMC data of server %q: %w", name, err)
	}

	var deployedServer provisioning.Server

	err = transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server %q by name: %w", name, err)
		}

		if server.Status != api.ServerStatusUnregistered {
			return fmt.Errorf("Server %q is in status %q, only an unregistered server can be deployed: %w", name, server.Status, domain.ErrOperationNotPermitted)
		}

		if !server.BMCConfig.HasBMC() {
			return fmt.Errorf("Server %q has no BMC configured: %w", name, domain.ErrOperationNotPermitted)
		}

		if server.StatusInternal.Deployment.IsActive() {
			return fmt.Errorf("Server %q is already being deployed (%s): %w", name, server.StatusInternal.Deployment.State, domain.ErrOperationNotPermitted)
		}

		token, err := s.tokenSvc.GetByUUID(ctx, request.TokenUUID)
		if err != nil {
			return fmt.Errorf("Failed to get token %q: %w", request.TokenUUID, err)
		}

		if token.UsesRemaining < 1 {
			return fmt.Errorf("Token %q has no uses remaining: %w", request.TokenUUID, domain.ErrOperationNotPermitted)
		}

		if token.ExpireAt.Before(s.now().Add(config.ServerDeploymentTimeout)) {
			return fmt.Errorf("Token %q expires at %s, which does not cover the deployment timeout of %s, so the server would not be able to register: %w", request.TokenUUID, token.ExpireAt.Format(time.RFC3339), config.ServerDeploymentTimeout, domain.ErrOperationNotPermitted)
		}

		seed, err := s.tokenSvc.GetTokenSeedByName(ctx, request.TokenUUID, request.Seed)
		if err != nil {
			return fmt.Errorf("Failed to get token seed %q: %w", request.Seed, err)
		}

		if !seed.Public {
			return fmt.Errorf("Token seed %q must be public to be used as installation media via the BMC: %w", request.Seed, domain.ErrOperationNotPermitted)
		}

		forceReboot := seed.Seeds.Install.ForceReboot
		if !forceReboot && !request.Force {
			return fmt.Errorf(`Token seed %q does not set "force_reboot", so the server does not reboot on its own when the installation is done. Re-run with force to rely on the read progress of the installation media alone: %w`, request.Seed, domain.ErrOperationNotPermitted)
		}

		if request.Channel != "" {
			_, err = s.channelSvc.GetByName(ctx, request.Channel)
			if err != nil {
				return fmt.Errorf("Failed to get channel %q: %w", request.Channel, err)
			}
		}

		if request.VirtualMediaID == "" {
			request.VirtualMediaID, err = selectVirtualMediaID(server.BMCData, request.ImageType, !forceReboot)
			if err != nil {
				return fmt.Errorf("Failed to select a virtual media device of server %q: %w", name, err)
			}
		} else {
			media, ok := server.BMCData.VirtualMedia[request.VirtualMediaID]
			if !ok {
				return fmt.Errorf("Server %q has no virtual media device %q, the BMC reports %s: %w", name, request.VirtualMediaID, describeVirtualMedia(server.BMCData), domain.ErrOperationNotPermitted)
			}

			if !virtualMediaSupportsImageType(media, request.ImageType) {
				return fmt.Errorf(
					"Virtual media device %q of server %q does not accept a %q image, it supports %s: %w",
					request.VirtualMediaID, name, request.ImageType, strings.Join(media.MediaTypes, ", "), domain.ErrOperationNotPermitted,
				)
			}
		}

		if !forceReboot && virtualMediaUploads(server.BMCData.VirtualMedia[request.VirtualMediaID]) {
			return deploymentUploadedMediaError(name, request.VirtualMediaID)
		}

		resolution, err := s.resolveBIOSProfile(ctx, *server)
		if err != nil {
			return err
		}

		if resolution == nil {
			return fmt.Errorf("No BIOS profile matches server %q: %w", name, domain.ErrNotFound)
		}

		now := s.now()

		server.StatusInternal.Deployment = &provisioning.ServerDeployment{
			State:                  api.ServerDeploymentStateRefreshBMCData,
			Request:                request,
			ForceReboot:            forceReboot,
			BIOSProfiles:           resolution.Profiles,
			BIOSAttributes:         maps.Clone(resolution.Attributes),
			BIOSDeferredAttributes: maps.Clone(resolution.DeferredAttributes),
			SecureBoot:             resolution.SecureBoot.Clone(),
			BIOSPending:            len(resolution.Attributes) > 0,
			BIOSDeferredPending:    len(resolution.DeferredAttributes) > 0,
			MediaBytesRead:         -1,
			StartedAt:              now,
			StateEnteredAt:         now,
			History:                []api.ServerDeploymentStep{},
		}

		server.Status = api.ServerStatusDeploying
		server.StatusDetail = api.ServerStatusDetailDeployingPreparing
		server.LastStatusUpdated = now

		err = server.Validate()
		if err != nil {
			return err
		}

		err = s.repo.Update(ctx, *server)
		if err != nil {
			return err
		}

		deployedServer = *server

		return nil
	})
	if err != nil {
		return err
	}

	deployedServer.SignalLifecycleEvent()

	return nil
}

// CancelDeploymentByName asks a running deployment to stop. In contrast to a
// failed deployment, a cancelled one is cleaned up: the installation media is
// ejected and the server is powered off. With skipCleanup, the deployment is
// stopped right away and the server is left untouched instead.
func (s *serverService) CancelDeploymentByName(ctx context.Context, name string, skipCleanup bool) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	var cancelledServer provisioning.Server

	err := transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server %q by name: %w", name, err)
		}

		deployment := server.StatusInternal.Deployment
		if !deployment.IsActive() {
			return fmt.Errorf("Server %q has no deployment in progress: %w", name, domain.ErrNotFound)
		}

		if deployment.CancelRequested {
			return nil
		}

		deployment.CancelRequested = true
		deployment.CancelSkipCleanup = skipCleanup

		err = s.repo.Update(ctx, *server)
		if err != nil {
			return err
		}

		cancelledServer = *server

		return nil
	})
	if err != nil {
		return err
	}

	// Interrupt the step in flight, so the cancellation does not have to wait out
	// a BMC, that is not answering anymore.
	run := s.lookupDeploymentRun(name)
	if run != nil {
		run.cancel()
	}

	cancelledServer.SignalLifecycleEvent()

	return nil
}

// DeploymentControlLoop advances the automated deployment of every server
// matching the filter and that has one in progress.
func (s *serverService) DeploymentControlLoop(ctx context.Context, serverNameFilter *string) error {
	names, err := s.deploymentCandidates(ctx, serverNameFilter)
	if err != nil {
		return fmt.Errorf("Failed to get servers for the deployment control loop: %w", err)
	}

	// Every step blocks on the BMC of its server, so the deployments are advanced
	// independently of each other: an unresponsive BMC must not keep the other
	// servers from moving on.
	var (
		errsMu sync.Mutex
		errs   []error
		wg     sync.WaitGroup
	)

	slots := make(chan struct{}, config.ServerDeploymentControlLoopConcurrency)

	for _, name := range names {
		slots <- struct{}{}

		wg.Go(func() {
			defer func() { <-slots }()

			err := s.runDeployment(ctx, name)
			if err == nil {
				return
			}

			errsMu.Lock()
			defer errsMu.Unlock()

			errs = append(errs, fmt.Errorf("Failed to advance the deployment of server %q: %w", name, err))
		})
	}

	wg.Wait()

	return errors.Join(errs...)
}

// deploymentCandidates returns the names of the servers, that have a deployment
// in progress.
func (s *serverService) deploymentCandidates(ctx context.Context, serverNameFilter *string) ([]string, error) {
	if serverNameFilter == nil {
		return s.repo.GetAllNamesWithActiveDeployment(ctx)
	}

	server, err := s.repo.GetByName(ctx, *serverNameFilter)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}

		return nil, err
	}

	if !server.StatusInternal.Deployment.IsActive() {
		return nil, nil
	}

	return []string{server.Name}, nil
}

// runDeployment advances the deployment of a single server as far as it gets
// without blocking, so a trigger and the wait it enables are handled within the
// same tick.
func (s *serverService) runDeployment(ctx context.Context, name string) error {
	run := s.acquireDeploymentRun(name)
	defer s.releaseDeploymentRun(name, run)

	ok := run.mu.TryLock()
	if !ok {
		// The deployment is already being advanced. Hand the trigger over to the
		// holder instead of dropping it, so an event, that satisfies the wait it
		// is in, is not waited out by the next tick.
		run.recordTrigger()

		return nil
	}

	defer run.mu.Unlock()

	run.takeTrigger()

	// Every transition of the tick is spent, before a trigger, that arrived
	// meanwhile, is picked up, so the re-run can not turn into a spin.
	for range config.ServerDeploymentMaxTransitionsPerTick {
		progressed, err := s.deploymentStep(ctx, name)
		if err != nil {
			return err
		}

		if !progressed && !run.takeTrigger() {
			return nil
		}
	}

	return nil
}

// deploymentStep performs a single step of the deployment of a server. It
// reports, whether the deployment advanced, so the caller can immediately
// evaluate the state entered.
func (s *serverService) deploymentStep(ctx context.Context, name string) (bool, error) {
	server, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return false, err
	}

	deployment := server.StatusInternal.Deployment
	if !deployment.IsActive() {
		return false, nil
	}

	now := s.now()

	log := slog.With(
		slog.String("name", name),
		slog.String("deployment_state", deployment.State.String()),
	)

	// An unknown state yields the zero definition, which is not a cancel phase and
	// not a kind the dispatcher drives, so it reaches the check below.
	definition, ok := deploymentStates[deployment.State]

	// Cancellation preempts everything but the clean up it triggers itself. A
	// cancellation, that skips the clean up, has nothing to trigger and ends the
	// deployment right here.
	if deployment.CancelRequested && !definition.cancelPhase {
		log.InfoContext(ctx, "Deployment cancelled", slog.Bool("skip_cleanup", deployment.CancelSkipCleanup))

		next := api.ServerDeploymentStateCancel
		if deployment.CancelSkipCleanup {
			next = api.ServerDeploymentStateCancelled
		}

		return true, s.advanceDeployment(ctx, name, next, nil)
	}

	if !deployment.CancelRequested && now.Sub(deployment.StartedAt) > config.ServerDeploymentTimeout {
		return false, s.failDeployment(ctx, name, fmt.Errorf("Deployment did not complete within %s", config.ServerDeploymentTimeout))
	}

	if !ok {
		return false, s.failDeployment(ctx, name, fmt.Errorf("Deployment is in the unknown state %q", deployment.State))
	}

	switch definition.kind {
	case deploymentStateKindAction:
		return s.deploymentAction(ctx, log, *server, definition)

	case deploymentStateKindWait:
		return s.deploymentWait(ctx, log, *server, definition)
	}

	return false, nil
}

func (s *serverService) deploymentAction(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, error) {
	deployment := server.StatusInternal.Deployment
	now := s.now()

	// Gate a retry on the backoff, so a BMC, that is rejecting requests, is not
	// hammered.
	if deployment.Retries > 0 && now.Before(deployment.LastAttemptAt.Add(deploymentBackoff(deployment.Retries))) {
		return false, nil
	}

	// Record the attempt of a retry before performing it, so the backoff above
	// measures from it. The first attempt of a state is not recorded, since
	// nothing reads it and a failure stamps it anyway. A crash in between leaves
	// the deployment in the trigger state, so the action is simply re-issued,
	// which every action tolerates: the BMC adapter establishes the precondition
	// of an operation before issuing it.
	if deployment.Retries > 0 {
		err := s.updateDeployment(ctx, server.Name, func(deployment *provisioning.ServerDeployment) {
			deployment.LastAttemptAt = now
		})
		if err != nil {
			return false, err
		}
	}

	// Record, that the action is about to be performed, before it is, so a
	// re-issued attempt knows about the side effect of the one before it, which
	// the BMC does not report anymore.
	if definition.prepare != nil {
		err := s.updateDeployment(ctx, server.Name, definition.prepare)
		if err != nil {
			return false, err
		}
	}

	log.InfoContext(ctx, "Deployment action triggered", slog.Int("retries", deployment.Retries))

	mutate, err := s.runBoundedDeploymentAction(ctx, log, server, definition)
	if err != nil {
		return false, s.recordDeploymentFailure(ctx, log, server.Name, definition, err)
	}

	return true, s.advanceDeployment(ctx, server.Name, definition.next, mutate)
}

func (s *serverService) deploymentWait(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, error) {
	deployment := server.StatusInternal.Deployment
	now := s.now()

	met, mutate, err := s.checkBoundedDeploymentWait(ctx, log, server, definition)
	if err != nil {
		fatal, ok := errors.AsType[deploymentFatalError](err)
		if ok {
			return false, s.failDeployment(ctx, server.Name, fatal)
		}

		// Failing to observe the condition is not failing the step, it is simply
		// repeated on the next tick until the state times out.
		log.WarnContext(ctx, "Failed to evaluate the deployment wait condition", logger.Err(err))

		return false, nil
	}

	if met {
		return true, s.advanceDeployment(ctx, server.Name, definition.next, mutate)
	}

	if now.Sub(deployment.StateEnteredAt) <= definition.timeout {
		if mutate == nil {
			return false, nil
		}

		return false, s.updateDeployment(ctx, server.Name, mutate)
	}

	timeoutErr := fmt.Errorf("Deployment state %q did not complete within %s", deployment.State, definition.timeout)

	if definition.fallback == "" {
		return false, s.failDeployment(ctx, server.Name, timeoutErr)
	}

	if deployment.Retries+1 > definition.retries {
		return false, s.failDeployment(ctx, server.Name, timeoutErr)
	}

	log.WarnContext(ctx, "Deployment wait timed out, falling back to the trigger", slog.String("fallback", definition.fallback.String()))

	return true, s.updateDeployment(ctx, server.Name, func(deployment *provisioning.ServerDeployment) {
		deployment.Retries++
		deployment.LastError = timeoutErr.Error()
		deployment.FallBackTo(now, definition.fallback)
	})
}

// deploymentForcePowerOff cuts the power instead of asking the server to shut
// down gracefully, since an ACPI request might get ignored depending on the
// stage the server is in.
const deploymentForcePowerOff = true

// runBoundedDeploymentAction performs the operation of a trigger state with a
// deadline of its own, so a BMC, that accepts the connection and then stops
// answering, ends the attempt instead of parking the control loop. Only the
// operation is bounded, never what the caller records about it.
func (s *serverService) runBoundedDeploymentAction(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (func(*provisioning.ServerDeployment), error) {
	callCtx, cancel := s.deploymentStepContext(ctx, server.Name, definition.callTimeoutOrDefault())
	defer cancel()

	return s.runDeploymentAction(callCtx, log, server, definition)
}

// checkBoundedDeploymentWait evaluates the condition of a wait state with the
// same deadline the trigger states get.
func (s *serverService) checkBoundedDeploymentWait(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	if definition.wait == nil {
		return false, nil, fmt.Errorf("Deployment state %q is not a wait", server.StatusInternal.Deployment.State)
	}

	callCtx, cancel := s.deploymentStepContext(ctx, server.Name, definition.callTimeoutOrDefault())
	defer cancel()

	return definition.wait(s, callCtx, log, server, definition)
}

// runDeploymentAction performs the operation of a trigger state and returns a
// mutation, that records what the operation produced.
func (s *serverService) runDeploymentAction(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	switch deployment.State {
	case api.ServerDeploymentStateRefreshBMCData:
		return nil, s.resyncBMCData(ctx, server)

	case api.ServerDeploymentStateCheckBIOS:
		return s.checkDeploymentBIOSAttributes(ctx, log, server, definition)

	case api.ServerDeploymentStatePowerOffBIOS, api.ServerDeploymentStatePowerOffBIOSDeferred,
		api.ServerDeploymentStatePowerOffSecureBoot, api.ServerDeploymentStatePowerOffSecureBootSettled:
		_, err := s.bmcServerPowerOffByName(ctx, server.Name, deploymentForcePowerOff, false)

		return nil, err

	case api.ServerDeploymentStateApplyBIOS, api.ServerDeploymentStateApplyBIOSDeferred:
		taskMonitor, err := s.applyBIOSAttributesByName(ctx, server.Name, deploymentBIOSAttributes(definition, deployment), false)
		if err != nil {
			return nil, err
		}

		return func(deployment *provisioning.ServerDeployment) {
			deployment.BIOSTaskMonitor = ""
			if taskMonitor != nil {
				deployment.BIOSTaskMonitor = taskMonitor.URI
			}
		}, nil

	case api.ServerDeploymentStatePowerOnBIOS, api.ServerDeploymentStatePowerOnBIOSDeferred,
		api.ServerDeploymentStatePowerOnSecureBoot, api.ServerDeploymentStatePowerOnInstall:
		_, err := s.bmcServerPowerOnByName(ctx, server.Name, false, false)

		return nil, err

	case api.ServerDeploymentStateVerifyBIOS, api.ServerDeploymentStateVerifyBIOSDeferred:
		return s.verifyDeploymentBIOSAttributes(ctx, log, server, definition)

	case api.ServerDeploymentStateSecureBoot:
		attempted := deployment.SecureBootAttempted

		enrolled, err := s.applySecureBootCertificatesByName(ctx, server.Name, deployment.SecureBoot)
		if err != nil {
			return nil, err
		}

		return func(deployment *provisioning.ServerDeployment) {
			deployment.SecureBootPending = enrolled || attempted
		}, nil

	case api.ServerDeploymentStateClearMedia:
		return nil, s.detachAllDeploymentMedia(ctx, server)

	case api.ServerDeploymentStateCleanup:
		return nil, s.cleanupDeploymentMedia(ctx, server)

	case api.ServerDeploymentStateAttachMedia:
		return s.attachDeploymentMedia(ctx, log, server, definition)

	case api.ServerDeploymentStateDetachMedia:
		_, err := s.bmcDetachMediaByName(ctx, server.Name, deployment.Request.VirtualMediaID, false)

		return nil, err

	case api.ServerDeploymentStateCancel:
		err := s.cleanupDeploymentMedia(ctx, server)
		if err != nil {
			return nil, err
		}

		_, err = s.bmcServerPowerOffByName(ctx, server.Name, deploymentForcePowerOff, false)

		return nil, err
	}

	return nil, fmt.Errorf("Deployment state %q is not an action", deployment.State)
}

// attachDeploymentMedia generates the installation media, attaches it and
// registers it as the boot device for the next boot.
func (s *serverService) attachDeploymentMedia(ctx context.Context, _ *slog.Logger, server provisioning.Server, _ deploymentStateDefinition) (func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment
	request := deployment.Request

	deploymentID := deployment.ImageDeploymentID
	if deploymentID == "" {
		deploymentID = provisioning.NewSeedImageDeploymentID()
	}

	if s.seedImageProgress != nil {
		s.seedImageProgress.Reset(ctx, deploymentID)
	}

	attached, err := s.bmcAttachMediaByName(ctx, server.Name, api.ServerBMCAttachMedia{
		TokenUUID:      request.TokenUUID.String(),
		Seed:           request.Seed,
		Type:           request.ImageType.String(),
		Architecture:   request.Architecture.String(),
		Channel:        request.Channel,
		VirtualMediaID: request.VirtualMediaID,
		SetBootDevice:  true,
	}, deploymentID, false)
	if err != nil {
		return nil, err
	}

	return func(deployment *provisioning.ServerDeployment) {
		deployment.MediaURL = attached.imageURL
		deployment.ImageDeploymentID = deploymentID
		deployment.MediaBytesRead = -1
		deployment.MediaSize = 0
	}, nil
}

// cleanupDeploymentMedia drops the read progress recorded for the installation
// media of this deployment and ejects it.
func (s *serverService) cleanupDeploymentMedia(ctx context.Context, server provisioning.Server) error {
	deployment := server.StatusInternal.Deployment

	if s.seedImageProgress != nil && deployment.ImageDeploymentID != "" {
		s.seedImageProgress.Reset(ctx, deployment.ImageDeploymentID)
	}

	return s.detachDeploymentMedia(ctx, server, deployment.Request.VirtualMediaID)
}

// detachAllDeploymentMedia ejects the media of every virtual media device, that
// reports something inserted.
func (s *serverService) detachAllDeploymentMedia(ctx context.Context, server provisioning.Server) error {
	return s.detachDeploymentMedia(ctx, server, "")
}

// detachDeploymentMedia ejects the media of every virtual media device, that
// reports something inserted, and of alsoDetach, if it is set.
func (s *serverService) detachDeploymentMedia(ctx context.Context, server provisioning.Server, alsoDetach string) error {
	current, err := s.deploymentBMCData(ctx, server)
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(current.BMCData.VirtualMedia)+1)

	for _, id := range slices.Sorted(maps.Keys(current.BMCData.VirtualMedia)) {
		if !current.BMCData.VirtualMedia[id].Inserted {
			continue
		}

		ids = append(ids, id)
	}

	if alsoDetach != "" && !slices.Contains(ids, alsoDetach) {
		ids = append(ids, alsoDetach)
	}

	var errs []error

	for _, id := range ids {
		_, err := s.bmcDetachMediaByName(ctx, server.Name, id, false)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func deploymentBIOSAttributes(definition deploymentStateDefinition, deployment *provisioning.ServerDeployment) map[string]any {
	if definition.biosPass == deploymentBIOSPassDeferred {
		return deployment.BIOSDeferredAttributes
	}

	return deployment.BIOSAttributes
}

func biosAttributeMatches(biosAttribute api.BIOSAttribute, want any) bool {
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(want)), strings.TrimSpace(fmt.Sprint(biosAttribute.CurrentValue)))
}

func (s *serverService) deploymentBIOSAttributesByName(ctx context.Context, server provisioning.Server) (map[string]api.BIOSAttribute, error) {
	biosAttributes, err := s.BMCBIOSAttributesByName(ctx, server.Name)
	if err != nil {
		return nil, err
	}

	current := make(map[string]api.BIOSAttribute, len(biosAttributes))
	for _, biosAttribute := range biosAttributes {
		current[biosAttribute.Name] = biosAttribute
	}

	return current, nil
}

// checkDeploymentBIOSAttributes records for both BIOS passes, whether they still
// have anything to apply, so an already correctly configured server is not power
// cycled for nothing. The outcome recorded for the second pass is only a first
// estimate, which the verification of the first pass overwrites, if it runs.
func (s *serverService) checkDeploymentBIOSAttributes(ctx context.Context, log *slog.Logger, server provisioning.Server, _ deploymentStateDefinition) (func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	current, err := s.deploymentBIOSAttributesByName(ctx, server)
	if err != nil {
		return nil, err
	}

	pending := deploymentBIOSPassPending(current, deployment.BIOSAttributes)
	if !pending && len(deployment.BIOSAttributes) > 0 {
		log.InfoContext(ctx, "BIOS attributes are applied already, passing the first BIOS pass by")
	}

	deferredPending := deploymentBIOSPassPending(current, deployment.BIOSDeferredAttributes)
	if !pending && !deferredPending && len(deployment.BIOSDeferredAttributes) > 0 {
		log.InfoContext(ctx, "Deferred BIOS attributes are applied already, passing the second BIOS pass by")
	}

	return func(deployment *provisioning.ServerDeployment) {
		deployment.BIOSPending = pending
		deployment.BIOSDeferredPending = deferredPending
	}, nil
}

// verifyDeploymentBIOSAttributes reads the BIOS attributes back and compares
// them to the set, the BIOS pass, that just ran, has applied. Attributes, that
// the BMC does not report at all, are skipped: not every attribute the firmware
// accepts is published back through the attribute registry.
func (s *serverService) verifyDeploymentBIOSAttributes(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	current, err := s.deploymentBIOSAttributesByName(ctx, server)
	if err != nil {
		return nil, err
	}

	deferredPass := definition.biosPass == deploymentBIOSPassDeferred

	applied := deploymentBIOSAttributes(definition, deployment)

	var mismatches []string

	for _, name := range slices.Sorted(maps.Keys(applied)) {
		want := applied[name]

		biosAttribute, ok := current[name]
		if !ok {
			log.WarnContext(ctx, "BIOS attribute is not reported back by the BMC, skipping its verification", slog.String("attribute", name))
			continue
		}

		if !biosAttributeMatches(biosAttribute, want) {
			mismatches = append(mismatches, fmt.Sprintf("%q is %q instead of %q", name, fmt.Sprint(biosAttribute.CurrentValue), fmt.Sprint(want)))
		}
	}

	if len(mismatches) > 0 {
		// A mismatch has to repeat the pass, that applied the attributes, power
		// cycle included: the firmware only picks the staged attributes up on the
		// next reset, so re-applying them without one changes nothing.
		return nil, deploymentRetryFromError{
			state: definition.retryFrom,
			err:   domain.NewRetryableErr(fmt.Errorf("BIOS attributes of server %q have not been applied: %s", server.Name, strings.Join(mismatches, ", "))),
		}
	}

	if deferredPass {
		return nil, nil
	}

	deferredPending := deploymentBIOSPassPending(current, deployment.BIOSDeferredAttributes)
	if !deferredPending && len(deployment.BIOSDeferredAttributes) > 0 {
		log.InfoContext(ctx, "Deferred BIOS attributes are applied already, passing the second BIOS pass by")
	}

	return func(deployment *provisioning.ServerDeployment) {
		deployment.BIOSDeferredPending = deferredPending
	}, nil
}

// deploymentBIOSPassPending reports, whether a BIOS pass still has anything to
// apply. An unreported attribute keeps the pass pending: not being able to tell
// has to run the pass, never skip it.
func deploymentBIOSPassPending(current map[string]api.BIOSAttribute, attributes map[string]any) bool {
	for name, want := range attributes {
		biosAttribute, ok := current[name]
		if !ok || !biosAttributeMatches(biosAttribute, want) {
			return true
		}
	}

	return false
}

// deploymentBMCWait turns a plain predicate over the BMC data of the server into
// a wait, so a state, whose condition is nothing but a look at the BMC, only has
// to name the predicate.
func deploymentBMCWait(condition func(*provisioning.ServerDeployment, api.BMCData) bool) deploymentWaitFunc {
	return func(s *serverService, ctx context.Context, _ *slog.Logger, server provisioning.Server, _ deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
		current, err := s.deploymentBMCData(ctx, server)
		if err != nil {
			return false, nil, err
		}

		return condition(server.StatusInternal.Deployment, current.BMCData), nil, nil
	}
}

func deploymentPowerIsOff(_ *provisioning.ServerDeployment, data api.BMCData) bool {
	return data.ServerPowerState == bmcPowerStateOff
}

// deploymentCancelSettled tells, whether the clean up of a cancelled deployment
// has come through.
func deploymentCancelSettled(deployment *provisioning.ServerDeployment, data api.BMCData) bool {
	return deploymentPowerIsOff(deployment, data) && deploymentNoMediaInserted(deployment, data)
}

func deploymentNoMediaInserted(_ *provisioning.ServerDeployment, data api.BMCData) bool {
	for _, media := range data.VirtualMedia {
		if media.Inserted {
			return false
		}
	}

	return true
}

func deploymentMediaHoldsImage(deployment *provisioning.ServerDeployment, data api.BMCData) bool {
	media, ok := data.VirtualMedia[deployment.Request.VirtualMediaID]

	return ok && media.Inserted && media.Image == deployment.MediaURL
}

func deploymentMediaEjected(deployment *provisioning.ServerDeployment, data api.BMCData) bool {
	media, ok := data.VirtualMedia[deployment.Request.VirtualMediaID]

	return !ok || !media.Inserted
}

func (s *serverService) checkDeploymentMediaAttached(ctx context.Context, _ *slog.Logger, server provisioning.Server, _ deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	current, err := s.deploymentBMCData(ctx, server)
	if err != nil {
		return false, nil, err
	}

	media, ok := current.BMCData.VirtualMedia[deployment.Request.VirtualMediaID]
	if ok && !deployment.ForceReboot && virtualMediaUploads(media) {
		return false, nil, deploymentFatalError{err: deploymentUploadedMediaError(server.Name, deployment.Request.VirtualMediaID)}
	}

	return deploymentMediaHoldsImage(deployment, current.BMCData), nil, nil
}

// checkDeploymentRebooted tells, whether the server has come back up after the
// first stage of the installation, powering it on again, if it stayed off.
func (s *serverService) checkDeploymentRebooted(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	if serverHasRegistered(server) {
		return true, nil, nil
	}

	current, err := s.deploymentBMCData(ctx, server)
	if err != nil {
		return false, nil, err
	}

	if current.BMCData.ServerPowerState == bmcPowerStateOff {
		_, err = s.bmcServerPowerOnByName(ctx, server.Name, false, false)

		return false, nil, err
	}

	rebooted, observed := deploymentRebootObserved(s.now(), server.StatusInternal.Deployment, current.BMCData, definition.rebootWindow)
	if rebooted && !observed {
		log.InfoContext(ctx, "Reboot of the server could not be observed, falling back to the power state")
	}

	return rebooted, nil, nil
}

// checkDeploymentBIOSApplied tells, whether the firmware has picked the staged
// BIOS attributes up.
func (s *serverService) checkDeploymentBIOSApplied(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	client, ok := s.bmcServerClients[server.BMCConfig.APIType]
	if !ok {
		return false, nil, fmt.Errorf("Failed to get BMC server client for type %q", server.BMCConfig.APIType)
	}

	var taskMonitor *provisioning.BMCTaskMonitor
	if deployment.BIOSTaskMonitor != "" {
		taskMonitor = &provisioning.BMCTaskMonitor{URI: deployment.BIOSTaskMonitor}
	}

	taskState, err := client.TaskState(ctx, server, taskMonitor)
	if err != nil {
		return false, nil, err
	}

	switch taskState {
	case api.BMCTaskStateCompleted:
		return true, nil, nil

	case api.BMCTaskStateRunning:
		return false, nil, nil
	}

	// The BMC does not know the task monitor anymore, which happens once it has
	// been consumed or after a BMC reset. Fall through to the observable
	// condition: the server is up again and had time to run through its POST.
	if s.now().Sub(deployment.StateEnteredAt) < definition.settleDelay {
		return false, nil, nil
	}

	current, err := s.deploymentBMCData(ctx, server)
	if err != nil {
		return false, nil, err
	}

	if current.BMCData.ServerPowerState != bmcPowerStateOn {
		return false, nil, nil
	}

	log.InfoContext(ctx, "BMC does not know the BIOS task monitor anymore, falling back to the power state")

	return true, nil, nil
}

// deploymentSettleSnapshot records the reboot relevant BMC properties, once the
// server is up and has reached a stable boot progress, so the power on, that
// starts a step, is not mistaken for the reboot, that ends it. It returns nil,
// as long as the server has not settled yet.
func deploymentSettleSnapshot(now time.Time, deployment *provisioning.ServerDeployment, current api.BMCData, settleDelay time.Duration, set func(*provisioning.ServerDeployment, provisioning.ServerDeploymentBMCSnapshot)) func(*provisioning.ServerDeployment) {
	if current.ServerPowerState != bmcPowerStateOn || now.Sub(deployment.StateEnteredAt) < settleDelay {
		return nil
	}

	snapshot := provisioning.NewServerDeploymentBMCSnapshot(now, current)

	return func(deployment *provisioning.ServerDeployment) {
		set(deployment, snapshot)
	}
}

// checkDeploymentSecureBootSettled tells, whether the firmware has picked the
// enrolled secure boot certificates up.
func (s *serverService) checkDeploymentSecureBootSettled(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	current, err := s.deploymentBMCData(ctx, server)
	if err != nil {
		return false, nil, err
	}

	now := s.now()

	if deployment.SecureBootSnapshot.Taken.IsZero() {
		return false, deploymentSettleSnapshot(now, deployment, current.BMCData, definition.settleDelay, func(deployment *provisioning.ServerDeployment, snapshot provisioning.ServerDeploymentBMCSnapshot) {
			deployment.SecureBootSnapshot = snapshot
		}), nil
	}

	if deployment.SecureBootSnapshot.HasRebootedSince(current.BMCData) == api.BMCRebootStateRebooted {
		log.InfoContext(ctx, "Firmware has picked the enrolled secure boot certificates up, the BMC reports a reboot of the server")

		return true, nil, nil
	}

	if now.Sub(deployment.StateEnteredAt) < definition.rebootWindow {
		return false, nil, nil
	}

	log.InfoContext(ctx, "Firmware did not reboot after the secure boot certificates were enrolled, continuing with the installation")

	return true, nil, nil
}

// checkDeploymentInstalled tells, whether the first stage of the IncusOS
// installation is done, from the strongest signal available.
func (s *serverService) checkDeploymentInstalled(ctx context.Context, log *slog.Logger, server provisioning.Server, definition deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	deployment := server.StatusInternal.Deployment

	// 1. The server registered itself, so it rebooted and finished on its own.
	if serverHasRegistered(server) {
		log.InfoContext(ctx, "Installation completed, the server has registered itself")

		return true, nil, nil
	}

	current, err := s.deploymentBMCData(ctx, server)
	if err != nil {
		return false, nil, err
	}

	now := s.now()

	if deployment.InstallSnapshot.Taken.IsZero() {
		return false, deploymentSettleSnapshot(now, deployment, current.BMCData, definition.settleDelay, func(deployment *provisioning.ServerDeployment, snapshot provisioning.ServerDeploymentBMCSnapshot) {
			deployment.InstallSnapshot = snapshot
		}), nil
	}

	// The read progress of the installation media is what tells the installation
	// apart from the server merely booting, so both remaining signals consult it.
	progress, progressKnown := s.deploymentMediaProgress(ctx, *current)

	bytesRead := int64(-1)
	if progressKnown {
		bytesRead = progress.BytesCovered
	}

	// The installer having been running is what tells the reboot at the end of
	// the first stage apart from the one the firmware performs to pick up, what
	// has been staged for it, since the latter comes within the POST cycles of
	// the very boot, that is supposed to start the installer.
	osObserved := deployment.InstallOSObserved || deploymentInstallOSObserved(deployment, current.BMCData)

	var mutate func(*provisioning.ServerDeployment)
	if bytesRead != deployment.MediaBytesRead || osObserved != deployment.InstallOSObserved {
		mutate = func(deployment *provisioning.ServerDeployment) {
			deployment.MediaBytesRead = bytesRead
			deployment.MediaSize = progress.Size
			deployment.InstallOSObserved = osObserved
		}
	}

	// 2. The BMC reports, that the server has rebooted since.
	rebootState := deployment.InstallSnapshot.HasRebootedSince(current.BMCData)
	if rebootState == api.BMCRebootStateRebooted {
		// The firmware reboots the server on its own, once it has applied the
		// staged BIOS attributes or picked the enrolled secure boot certificates
		// up, both of which happen on the very boot, that is supposed to start
		// the installer, before it ever gets to run. Such a reboot re-anchors the
		// detection instead of ending the wait.
		if !deploymentInstallRebootTellsTheInstallation(now, deployment, osObserved, definition.install) {
			log.InfoContext(
				ctx, "Server rebooted before the installation could have completed, waiting for the installation",
				slog.Bool("media_progress_known", progressKnown),
				slog.Int64("bytes_covered", progress.BytesCovered),
				slog.Int64("bytes_served", progress.BytesServed),
				slog.Int64("bytes_required", deploymentMediaBytesRequired(progress.Size, definition.install.mediaMinBytesRead)),
				slog.Duration("installing_for", now.Sub(deployment.StateEnteredAt)),
			)

			snapshot := provisioning.NewServerDeploymentBMCSnapshot(now, current.BMCData)

			return false, func(deployment *provisioning.ServerDeployment) {
				if mutate != nil {
					mutate(deployment)
				}

				deployment.InstallSnapshot = snapshot
			}, nil
		}

		log.InfoContext(
			ctx, "Installation completed, the BMC reports a reboot of the server",
			slog.Bool("os_observed", osObserved),
			slog.Bool("media_read_out", progressKnown && deploymentMediaReadOut(progress, definition.install.mediaMinBytesRead)),
			slog.Int64("bytes_covered", progress.BytesCovered),
			slog.Duration("installing_for", now.Sub(deployment.StateEnteredAt)),
		)

		return true, mutate, nil
	}

	// 3. The BMC has read enough of the installation media and stopped reading.
	//    Only a deployment, whose server does not reboot on its own, is told by
	//    the read progress: a BMC, that caches the media instead of handing every
	//    read through to the installer, reads it out while the server boots and
	//    then goes quiet, which is indistinguishable from an installation, that
	//    has been read out and is done.
	if !deployment.ForceReboot && deploymentInstallCouldBeDone(now, deployment, definition.install.minDuration) && progressKnown && deploymentMediaReadOut(progress, definition.install.mediaMinBytesRead) && deploymentMediaIdle(now, progress, definition.install.mediaIdlePeriod) {
		log.InfoContext(ctx, "Installation completed, the installation media has been read and is idle", slog.Int64("bytes_covered", progress.BytesCovered))

		return true, mutate, nil
	}

	return false, mutate, nil
}

// deploymentMediaProgress returns the read progress recorded for the
// installation media of the deployment and reports, whether it can be relied on
// at all. A daemon restart loses the in memory progress and a BMC, that uploads
// the media instead of streaming it, reads it before the server even boots.
// Both are reported as "cannot tell", so the caller falls back to the other
// signals instead of waiting for a progress, that will never come.
func (s *serverService) deploymentMediaProgress(ctx context.Context, server provisioning.Server) (provisioning.SeedImageProgress, bool) {
	deployment := server.StatusInternal.Deployment

	if s.seedImageProgress == nil || deployment.ImageDeploymentID == "" {
		return provisioning.SeedImageProgress{}, false
	}

	media, ok := server.BMCData.VirtualMedia[deployment.Request.VirtualMediaID]
	if ok && virtualMediaUploads(media) {
		return provisioning.SeedImageProgress{}, false
	}

	progress, ok := s.seedImageProgress.Get(ctx, deployment.ImageDeploymentID)
	if !ok {
		slog.DebugContext(
			ctx, "The BMC has not read anything of the installation media yet",
			slog.String("name", server.Name),
			slog.String("deployment_id", deployment.ImageDeploymentID),
		)

		return provisioning.SeedImageProgress{}, false
	}

	return progress, true
}

// deploymentInstallCouldBeDone reports, whether the first stage of the
// installation can be done at all. A firmware, that still has something to pick
// up, reboots within the first POST cycles and keeps the media quiet meanwhile,
// while the installation itself takes considerably longer.
func deploymentInstallCouldBeDone(now time.Time, deployment *provisioning.ServerDeployment, minDuration time.Duration) bool {
	return now.Sub(deployment.StateEnteredAt) >= minDuration
}

// deploymentInstallRebootTellsTheInstallation reports, whether a reboot the BMC
// observed can be taken as the end of the first stage of the installation.
func deploymentInstallRebootTellsTheInstallation(now time.Time, deployment *provisioning.ServerDeployment, osObserved bool, thresholds deploymentInstallThresholds) bool {
	if osObserved {
		return true
	}

	return now.Sub(deployment.StateEnteredAt) >= thresholds.rebootFallbackDelay
}

// deploymentInstallOSObserved reports, whether the BMC has the server past the
// hand over to the operating system, which the installer, and only a boot, that
// got as far as starting it, reaches. A state, that was already reported when
// the install wait was anchored, belongs to an earlier boot and says nothing
// about this one.
func deploymentInstallOSObserved(deployment *provisioning.ServerDeployment, current api.BMCData) bool {
	progress := current.ServerBootProgress

	if !api.BMCBootProgressHandedOverToOS(progress.LastState) {
		return false
	}

	return !progress.LastStateTime.Before(deployment.InstallSnapshot.Taken)
}

// deploymentMediaReadOut reports, whether enough of the installation media has
// been read to tell the installer apart from the server merely booting. It
// counts the distinct bytes served, since a BMC re-requesting ranges it has
// fetched before must not be mistaken for the installer streaming the image.
func deploymentMediaReadOut(progress provisioning.SeedImageProgress, minBytesRead int64) bool {
	return progress.BytesCovered >= deploymentMediaBytesRequired(progress.Size, minBytesRead)
}

func deploymentMediaIdle(now time.Time, progress provisioning.SeedImageProgress, idlePeriod time.Duration) bool {
	return progress.IdleFor(now) >= idlePeriod
}

// deploymentMediaBytesRequired returns how much of the installation media has to
// have been read, before the installation counts as under way. It is capped at
// the size of the image, so a smaller media is not excluded from the signal.
func deploymentMediaBytesRequired(size int64, minBytesRead int64) int64 {
	if size > 0 && size < minBytesRead {
		return size
	}

	return minBytesRead
}

// deploymentBMCData returns the server with its BMC data refreshed, unless it is
// fresh enough already.
func (s *serverService) deploymentBMCData(ctx context.Context, server provisioning.Server) (*provisioning.Server, error) {
	if !server.BMCData.LastUpdated.IsZero() && s.now().Sub(server.BMCData.LastUpdated) < config.ServerDeploymentControlLoopInterval {
		return &server, nil
	}

	err := s.resyncBMCData(ctx, server)
	if err != nil {
		return nil, err
	}

	return s.repo.GetByName(ctx, server.Name)
}

// deploymentRebootObserved reports, whether the server can be taken to have
// rebooted after the first stage of the installation, and whether the reboot has
// actually been observed rather than merely assumed. The reboot is measured
// against the snapshot taken for the install wait, so a reboot, that already
// ended that wait, is recognized right away. A BMC, that reports none of the
// properties the detection needs, can never answer the question, so the
// observation window bounds the wait instead of failing the deployment.
func deploymentRebootObserved(now time.Time, deployment *provisioning.ServerDeployment, current api.BMCData, rebootWindow time.Duration) (rebooted bool, observed bool) {
	if deployment.InstallSnapshot.Taken.IsZero() {
		return true, false
	}

	if deployment.InstallSnapshot.HasRebootedSince(current) == api.BMCRebootStateRebooted {
		return true, true
	}

	if now.Sub(deployment.StateEnteredAt) < rebootWindow {
		return false, false
	}

	return true, false
}

// checkDeploymentRegistered tells, whether the server has registered itself with
// Operations Center after the installation.
func (*serverService) checkDeploymentRegistered(_ context.Context, _ *slog.Logger, server provisioning.Server, _ deploymentStateDefinition) (bool, func(*provisioning.ServerDeployment), error) {
	return serverHasRegistered(server), nil, nil
}

func serverHasRegistered(server provisioning.Server) bool {
	switch server.Status {
	case api.ServerStatusPending, api.ServerStatusReady:
		return true
	}

	return false
}

// advanceDeployment applies, what the completed step produced, and enters the
// state it leads to, passing by the states with nothing left to do.
func (s *serverService) advanceDeployment(ctx context.Context, name string, next api.ServerDeploymentState, mutate func(*provisioning.ServerDeployment)) error {
	return s.updateDeployment(ctx, name, func(deployment *provisioning.ServerDeployment) {
		if mutate != nil {
			mutate(deployment)
		}

		deployment.EnterState(s.now(), deploymentNextState(deployment, next))
	})
}

// recordDeploymentFailure accounts for a failed step, either by scheduling
// another attempt or by failing the deployment.
func (s *serverService) recordDeploymentFailure(ctx context.Context, log *slog.Logger, name string, definition deploymentStateDefinition, stepErr error) error {
	server, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return err
	}

	deployment := server.StatusInternal.Deployment
	if !deployment.IsActive() {
		return nil
	}

	// A step, that routes the deployment back to an earlier state, gets its own
	// budget: the state it falls back to succeeds every time, so a shared retry
	// counter would be reset on every round and the two states would loop.
	retryFrom, ok := errors.AsType[deploymentRetryFromError](stepErr)
	if ok {
		if deployment.FallbackAttempts+1 > definition.retries {
			return s.failDeployment(ctx, name, stepErr)
		}

		log.WarnContext(ctx, "Deployment step failed, going back to an earlier state", logger.Err(stepErr), slog.String("state", retryFrom.state.String()), slog.Int("attempts", deployment.FallbackAttempts+1))

		return s.updateDeployment(ctx, name, func(deployment *provisioning.ServerDeployment) {
			deployment.FallbackAttempts++
			deployment.LastError = stepErr.Error()
			deployment.LastAttemptAt = s.now()
			deployment.FallBackTo(s.now(), retryFrom.state)
		})
	}

	if !domain.IsRetryableError(stepErr) || deployment.Retries+1 > definition.retries {
		return s.failDeployment(ctx, name, stepErr)
	}

	log.WarnContext(ctx, "Deployment step failed, scheduling another attempt", logger.Err(stepErr), slog.Int("retries", deployment.Retries+1))

	return s.updateDeployment(ctx, name, func(deployment *provisioning.ServerDeployment) {
		deployment.Retries++
		deployment.LastError = stepErr.Error()
		deployment.LastAttemptAt = s.now()
	})
}

// failDeployment ends a deployment, recording the state it failed in and the
// error. Nothing is cleaned up on purpose, so an operator can look at the server
// through the BMC console.
func (s *serverService) failDeployment(ctx context.Context, name string, stepErr error) error {
	slog.ErrorContext(ctx, "Deployment failed", slog.String("name", name), logger.Err(stepErr))

	return s.updateDeployment(ctx, name, func(deployment *provisioning.ServerDeployment) {
		deployment.LastError = stepErr.Error()
		deployment.FailedState = deployment.State
		deployment.EnterState(s.now(), api.ServerDeploymentStateFailed)
		deployment.LastError = stepErr.Error()
	})
}

// updateDeployment applies a change to the deployment of a server and keeps the
// server status in sync with it.
func (s *serverService) updateDeployment(ctx context.Context, name string, mutate func(*provisioning.ServerDeployment)) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return err
		}

		if server.StatusInternal.Deployment == nil {
			return fmt.Errorf("Server %q has no deployment: %w", name, domain.ErrNotFound)
		}

		mutate(server.StatusInternal.Deployment)

		s.applyDeploymentServerStatus(server)

		return s.repo.Update(ctx, *server)
	})
}

// applyDeploymentServerStatus reports the progress of the deployment through the
// server status and status detail, which is also how the control loop finds its
// work again. The status is only ever touched, while the server is in status
// deploying: once it has registered itself, it owns its status again.
func (s *serverService) applyDeploymentServerStatus(server *provisioning.Server) {
	deployment := server.StatusInternal.Deployment

	if server.Status != api.ServerStatusDeploying {
		return
	}

	definition := deploymentStates[deployment.State]

	status := definition.status
	if status == "" {
		status = api.ServerStatusDeploying
	}

	statusDetail := definition.detail

	if server.Status == status && server.StatusDetail == statusDetail {
		return
	}

	server.Status = status
	server.StatusDetail = statusDetail
	server.LastStatusUpdated = s.now()
}

const (
	bmcPowerStateOn                  = string(schemas.OnPowerState)
	bmcPowerStateOff                 = string(schemas.OffPowerState)
	virtualMediaTransferMethodUpload = string(schemas.UploadTransferMethod)
)

// virtualMediaTypesByImageType holds the virtual media types able to hold an
// image of a given type. It mirrors, what the BMC adapter derives from the
// extension of the media URL, so a device, that the attachment would reject, is
// not picked in the first place.
var virtualMediaTypesByImageType = map[api.ImageType][]string{
	api.ImageTypeISO: {string(schemas.CDVirtualMediaType), string(schemas.DVDVirtualMediaType)},
	api.ImageTypeRaw: {string(schemas.USBStickVirtualMediaType), string(schemas.FloppyVirtualMediaType)},
}

var virtualMediaServicePrecedence = []string{"system", "manager"}

// virtualMediaUploads reports, whether a virtual media device pulls the image in
// before the server boots, instead of streaming.
func virtualMediaUploads(media api.BMCVirtualMedia) bool {
	return strings.EqualFold(media.TransferMethod, virtualMediaTransferMethodUpload)
}

func deploymentUploadedMediaError(name string, virtualMediaID string) error {
	return fmt.Errorf(
		`Virtual media device %q of server %q uploads the installation media instead of streaming it, so its read progress can not tell, when the installation is done. Use a token seed, that sets "force_reboot", or a virtual media device, that streams: %w`,
		virtualMediaID, name, domain.ErrOperationNotPermitted,
	)
}

// virtualMediaAdvertisesImageType reports, whether a virtual media device says
// itself, that it can hold an image of the given type.
func virtualMediaAdvertisesImageType(media api.BMCVirtualMedia, imageType api.ImageType) bool {
	wanted := virtualMediaTypesByImageType[imageType]

	return slices.ContainsFunc(media.MediaTypes, func(mediaType string) bool {
		return slices.Contains(wanted, mediaType)
	})
}

// virtualMediaSupportsImageType reports, whether a virtual media device can hold
// an image of the given type. A device, that advertises no media type at all,
// leaves the decision to the BMC.
func virtualMediaSupportsImageType(media api.BMCVirtualMedia, imageType api.ImageType) bool {
	if len(media.MediaTypes) == 0 || len(virtualMediaTypesByImageType[imageType]) == 0 {
		return true
	}

	return virtualMediaAdvertisesImageType(media, imageType)
}

// selectVirtualMediaID picks the virtual media device the installation media is
// attached to, preferring a device advertising support for the requested image
// type over one advertising nothing and, among equally suitable devices, one
// offered by the system over one offered by the manager.
//
// A deployment, that has only the read progress of the installation media to
// tell the installation by, needs a device, that streams it.
func selectVirtualMediaID(data api.BMCData, imageType api.ImageType, requireStreaming bool) (string, error) {
	if len(data.VirtualMedia) == 0 {
		return "", fmt.Errorf("The BMC reports no virtual media device: %w", domain.ErrNotFound)
	}

	ids := virtualMediaIDsByPreference(data)

	if requireStreaming {
		streaming := slices.DeleteFunc(slices.Clone(ids), func(id string) bool {
			return virtualMediaUploads(data.VirtualMedia[id])
		})

		id, ok := selectVirtualMediaIDFrom(data, streaming, imageType)
		if ok {
			return id, nil
		}
	}

	id, ok := selectVirtualMediaIDFrom(data, ids, imageType)
	if ok {
		return id, nil
	}

	return "", fmt.Errorf(
		"No virtual media device of the BMC accepts a %q image, it reports %s: %w",
		imageType, describeVirtualMediaTypes(data), domain.ErrOperationNotPermitted,
	)
}

// selectVirtualMediaIDFrom picks the device holding an image of the given type
// out of ids, which the caller has already brought into the order of preference.
func selectVirtualMediaIDFrom(data api.BMCData, ids []string, imageType api.ImageType) (string, bool) {
	for _, id := range ids {
		if virtualMediaAdvertisesImageType(data.VirtualMedia[id], imageType) {
			return id, true
		}
	}

	// Nothing advertises the media type the image needs, so fall back to a
	// device, that does not say, what it takes.
	for _, id := range ids {
		if len(data.VirtualMedia[id].MediaTypes) == 0 {
			return id, true
		}
	}

	return "", false
}

// virtualMediaIDsByPreference returns the IDs of all virtual media devices, the
// ones offered by the system first, each service in a stable order.
func virtualMediaIDsByPreference(data api.BMCData) []string {
	ids := slices.Sorted(maps.Keys(data.VirtualMedia))

	slices.SortStableFunc(ids, func(a string, b string) int {
		return cmp.Compare(virtualMediaServiceRank(a), virtualMediaServiceRank(b))
	})

	return ids
}

// virtualMediaServiceRank ranks the service a virtual media ID names, an unknown
// service last.
func virtualMediaServiceRank(id string) int {
	service, _, _ := strings.Cut(id, ":")

	rank := slices.Index(virtualMediaServicePrecedence, service)
	if rank < 0 {
		return len(virtualMediaServicePrecedence)
	}

	return rank
}

func describeVirtualMedia(data api.BMCData) string {
	if len(data.VirtualMedia) == 0 {
		return "no virtual media device"
	}

	return strings.Join(slices.Sorted(maps.Keys(data.VirtualMedia)), ", ")
}

// describeVirtualMediaTypes names every virtual media device along with the
// media types it advertises, so an operator can tell, which device to ask for.
func describeVirtualMediaTypes(data api.BMCData) string {
	if len(data.VirtualMedia) == 0 {
		return "no virtual media device"
	}

	descriptions := make([]string, 0, len(data.VirtualMedia))

	for _, id := range slices.Sorted(maps.Keys(data.VirtualMedia)) {
		mediaTypes := "no media type"
		if len(data.VirtualMedia[id].MediaTypes) > 0 {
			mediaTypes = strings.Join(data.VirtualMedia[id].MediaTypes, ", ")
		}

		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", id, mediaTypes))
	}

	return strings.Join(descriptions, ", ")
}
