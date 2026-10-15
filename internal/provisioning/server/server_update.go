package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// claimRollingUpdateStep takes the claim on a step of a cluster wide run for the
// given server, which the caller is about to trigger.
func (s *serverService) claimRollingUpdateStep(server *provisioning.Server, step provisioning.ServerUpdateStep) error {
	serverUpdate := server.StatusInternal.Update
	if !serverUpdate.IsActive() {
		return nil
	}

	now := s.now()

	if serverUpdate.InFlight(now) {
		return domain.NewRetryableErr(fmt.Errorf("Step %q for server %q is in flight", step, server.Name))
	}

	retries := step.Retries()
	if serverUpdate.Step == step {
		if serverUpdate.Retries >= retries {
			return domain.NewErrorf(domain.ErrTerminal, "", "Failed to %s server %q in %d attempts: %s", step, server.Name, retries, serverUpdate.FirstError).
				WithHintf("Resolve the reported problem on server %q and start the update again.", server.Name).
				WithDetail("server", server.Name).
				WithDetail("step", string(step)).
				WithDetail("first_error", serverUpdate.FirstError).
				WithDetail("last_error", serverUpdate.LastError)
		}

		// A failed attempt is not retried immediately. The condition, which made
		// it fail, is often transient but needs longer to clear than the control
		// loop needs to pick the server up again.
		if serverUpdate.RetryBackoffRemaining(now, s.rollingUpdateStepRetryBackoff) > 0 {
			return domain.NewRetryableErr(
				domain.NewErrorf(nil, "", "Step %q for server %q backs off for %s after a failed attempt: %s", step, server.Name, s.rollingUpdateStepRetryBackoff, serverUpdate.LastError).
					WithDetail("server", server.Name).
					WithDetail("step", string(step)).
					WithDetail("last_error", serverUpdate.LastError),
			)
		}
	}

	serverUpdate.Claim(now, step)

	return nil
}

// stepCallback returns the callback for the outcome of a step, which runs on
// the server in the background. For a manually triggered step, it is the only
// thing that rewinds the transient state of the server.
func (s *serverService) stepCallback(name string, step provisioning.ServerUpdateStep, clusterUpdate bool, previousInMaintenance api.InMaintenanceState) func(ctx context.Context, err error) {
	// Evacuate and restore share the scope, so a later success of either one
	// clears the warning.
	scope := api.WarningScope{
		Scope:      "lifecycle_operation",
		EntityType: "server",
		Entity:     name,
	}

	return func(ctx context.Context, stepErr error) {
		// The error comes from the operation directly. It did not pass the error
		// wrapper middleware of the client.
		stepErr = domain.RetryableWrapper()(stepErr)
		if stepErr == nil {
			s.warning.RemoveStale(ctx, scope, nil)
			return
		}

		s.warning.Emit(ctx, warning.NewWarningFromError(api.WarningTypeServerLifecycleOperationFailed, scope, stepErr, "Failed to %s server %q", step, name))

		s.recordStepFailure(ctx, name, step, clusterUpdate, previousInMaintenance, stepErr)
	}
}

// recordStepFailure rewinds the status detail and the Incus maintenance state,
// which Operations Center set when it triggered the step. Only a step of a
// rolling update counts as a failed attempt of the run.
//
// The rewind only happens if the status detail still belongs to the step. This
// does not tell apart two attempts of the same step.
func (s *serverService) recordStepFailure(ctx context.Context, name string, step provisioning.ServerUpdateStep, clusterUpdate bool, previousInMaintenance api.InMaintenanceState, stepErr error) {
	var server *provisioning.Server
	err := transaction.Do(ctx, func(ctx context.Context) error {
		var err error

		server, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server %q by name: %w", name, err)
		}

		if clusterUpdate {
			server.StatusInternal.Update.Fail(s.now(), step, stepErr)
		}

		if step.OwnsStatusDetail(server.StatusDetail) {
			server.StatusDetail = api.ServerStatusDetailNone
			server.LastStatusUpdated = s.now()

			rewindIncusInMaintenance(server, previousInMaintenance)
		}

		return s.repo.Update(ctx, *server)
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to record the failure of a step", slog.String("server", name), slog.String("step", string(step)), logger.Err(err))
		return
	}

	server.SignalLifecycleEvent()
}

func rewindIncusInMaintenance(server *provisioning.Server, previousInMaintenance api.InMaintenanceState) {
	for i := range server.VersionData.Applications {
		application := &server.VersionData.Applications[i]
		if !domain.IsApplicationNameIncusKind(application.Name) {
			continue
		}

		// Polling may already have reported the actual state.
		if application.InMaintenance == api.InMaintenanceEvacuating || application.InMaintenance == api.InMaintenanceRestoring {
			application.InMaintenance = previousInMaintenance
		}

		return
	}
}

func incusInMaintenance(server provisioning.Server) api.InMaintenanceState {
	for _, application := range server.VersionData.Applications {
		if !domain.IsApplicationNameIncusKind(application.Name) {
			continue
		}

		// A rewind to a transient state would never resolve, so use the state
		// before the transition.
		switch application.InMaintenance {
		case api.InMaintenanceEvacuating:
			return api.NotInMaintenance

		case api.InMaintenanceRestoring:
			return api.InMaintenanceEvacuated

		default:
			return application.InMaintenance
		}
	}

	return api.NotInMaintenance
}

// BeginUpdateRunByCluster starts a cluster wide rolling update or rolling reboot
// for every server of the cluster.
func (s *serverService) BeginUpdateRunByCluster(ctx context.Context, clusterName string, rebootPending bool) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		servers, err := s.repo.GetAllWithFilter(ctx, provisioning.ServerFilter{
			Cluster: &clusterName,
		})
		if err != nil {
			return fmt.Errorf("Failed to get servers of cluster %q: %w", clusterName, err)
		}

		now := s.now()

		for _, server := range servers {
			triggered := server.StatusInternal.Update.TriggeredUpdate()

			server.StatusInternal.Update = &provisioning.ServerUpdate{
				Triggered:     triggered,
				StartedAt:     now,
				RebootPending: rebootPending || serverOwesReboot(server),
				KeepEvacuated: ptr.From(server.VersionData.InMaintenance) == api.InMaintenanceEvacuated,
			}

			if isTransientStatusDetail(server.StatusDetail) {
				server.StatusDetail = api.ServerStatusDetailNone
				server.LastStatusUpdated = now
			}

			err = s.repo.Update(ctx, server)
			if err != nil {
				return fmt.Errorf("Failed to start the update run for server %q: %w", server.Name, err)
			}
		}

		return nil
	})
}

// EndUpdateRunByCluster drops what the run has recorded about the servers of the
// cluster.
func (s *serverService) EndUpdateRunByCluster(ctx context.Context, clusterName string) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		servers, err := s.repo.GetAllWithFilter(ctx, provisioning.ServerFilter{
			Cluster: &clusterName,
		})
		if err != nil {
			return fmt.Errorf("Failed to get servers of cluster %q: %w", clusterName, err)
		}

		for _, server := range servers {
			if !server.StatusInternal.Update.IsActive() {
				continue
			}

			server.StatusInternal.Update.EndRun()

			if server.StatusInternal.Update.IsEmpty() {
				server.StatusInternal.Update = nil
			}

			err = s.repo.Update(ctx, server)
			if err != nil {
				return fmt.Errorf("Failed to end the update run for server %q: %w", server.Name, err)
			}
		}

		return nil
	})
}

// serverOwesReboot reports, whether the server has an IncusOS update, that is
// staged but not booted yet.
func serverOwesReboot(server provisioning.Server) bool {
	if ptr.From(server.VersionData.NeedsReboot) {
		return true
	}

	return server.VersionData.OS.VersionNext != "" && server.VersionData.OS.VersionNext != server.VersionData.OS.Version
}

// isTransientStatusDetail reports, whether the status detail is one Operations
// Center sets while it waits for a step of a rolling update to complete and
// which it may rewind, when it launches a run.
func isTransientStatusDetail(statusDetail api.ServerStatusDetail) bool {
	switch statusDetail {
	case api.ServerStatusDetailReadyUpdatingOS,
		api.ServerStatusDetailReadyUpdatingApplication,
		api.ServerStatusDetailReadyEvacuating,
		api.ServerStatusDetailReadyRestoring:
		return true
	}

	return false
}
