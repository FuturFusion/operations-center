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
	if serverUpdate.Step == step && serverUpdate.Retries >= retries {
		return fmt.Errorf("Failed to %s server %q in %d attempts, lastErr: %s: %w", step, server.Name, retries, serverUpdate.LastError, domain.ErrTerminal)
	}

	serverUpdate.Claim(now, step)

	return nil
}

// recordRollingUpdateStepFailure accounts for a step, whose outcome is reported
// as failed, and rewinds the status detail Operations Center set when it
// triggered the step, so the control loop picks the server up again.
//
// The rewind is guarded on the status detail still being the one of the failed
// step, so a late report of an earlier attempt can not disturb the current one.
func (s *serverService) recordRollingUpdateStepFailure(ctx context.Context, name string, step provisioning.ServerUpdateStep, stepErr error) {
	err := transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server %q by name: %w", name, err)
		}

		server.StatusInternal.Update.Fail(step, stepErr)

		if step.OwnsStatusDetail(server.StatusDetail) {
			server.StatusDetail = api.ServerStatusDetailNone
			server.LastStatusUpdated = s.now()
		}

		return s.repo.Update(ctx, *server)
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to record the failure of a rolling update step", slog.String("server", name), slog.String("step", string(step)), logger.Err(err))
	}
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
