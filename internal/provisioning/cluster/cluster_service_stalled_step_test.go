package cluster_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// driveRollingUpdateIterations bounds the number of ticks, a driven run gets.
const driveRollingUpdateIterations = 200

// driveRollingUpdate ticks the control loop until the run of the cluster has
// finished, letting the fake servers complete the actions, that have been
// triggered on them. step, if set, is called on every iteration before the
// control loop runs, so a test can interfere with the run.
func driveRollingUpdate(t *testing.T, ctx context.Context, clusterSvc provisioning.ClusterService, world *serverWorld, step func(i int) bool) bool {
	t.Helper()

	for i := range driveRollingUpdateIterations {
		c, err := clusterSvc.GetByName(ctx, "clusterA")
		require.NoError(t, err)

		if c.UpdateStatus.InProgressStatus.InProgress == api.ClusterUpdateInProgressInactive {
			return true
		}

		require.Empty(t, c.UpdateStatus.InProgressStatus.Error)

		handled := false
		if step != nil {
			handled = step(i)
		}

		pending := world.pendingCount()

		err = clusterSvc.ClusterUpdateControlLoop(ctx, nil)
		if !domain.IsRetryableError(err) {
			require.NoError(t, err)
		}

		if !handled && pending > 0 {
			world.release(ctx)
		}

		time.Sleep(controlLoopInterval)
	}

	return false
}

func TestClusterService_ClusterUpdateControlLoopRetriggersStalledRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*100)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	serverClient := rollingUpdateServerClient(world)

	env := setupControlLoopEnv(t, ctx, server)
	clusterSvc, _ := newControlLoopServices(t, env, t.Name(), serverClient, "2")

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	stalled := false

	success := driveRollingUpdate(t, ctx, clusterSvc, world, func(i int) bool {
		// Lose the first restore: the member stays evacuated and nothing ever
		// reports what became of it.
		if !stalled && len(serverClient.RestoreCalls()) == 1 && world.pendingCount() > 0 {
			world.drop()
			env.clock.advance(config.ClusterRollingUpdateRestoreTimeout + time.Minute)
			stalled = true

			return true
		}

		return false
	})

	require.True(t, stalled, "the restore was never triggered")
	require.True(t, success, "the rolling update did not complete")

	// The lost restore was triggered again instead of parking the run.
	require.Len(t, serverClient.RestoreCalls(), 2)
}

func TestClusterService_ClusterUpdateControlLoopRetriggersStalledRestoreAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*100)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	serverClient := rollingUpdateServerClient(world)

	env := setupControlLoopEnv(t, ctx, server)
	clusterSvc, _ := newControlLoopServices(t, env, t.Name(), serverClient, "2")

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	restarted := false

	success := driveRollingUpdate(t, ctx, clusterSvc, world, func(i int) bool {
		if restarted || len(serverClient.RestoreCalls()) != 1 || world.pendingCount() == 0 {
			return false
		}

		// The goroutine waiting on the Incus operation dies with the process.
		world.drop()

		// Everything, that is kept in memory, is gone after the restart, the
		// database is not.
		clusterSvc, _ = newControlLoopServices(t, env, t.Name()+"Restarted", serverClient, "2")

		env.clock.advance(config.ClusterRollingUpdateRestoreTimeout + time.Minute)
		restarted = true

		return true
	})

	require.True(t, restarted, "the restore was never triggered")
	require.True(t, success, "the rolling update did not resume after the restart")

	require.Len(t, serverClient.RestoreCalls(), 2)
}

func TestClusterService_ClusterUpdateControlLoopRetriesTransientEvacuationFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*100)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	serverClient := rollingUpdateServerClient(world)

	env := setupControlLoopEnv(t, ctx, server)
	clusterSvc, _ := newControlLoopServices(t, env, t.Name(), serverClient, "2")

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	// The error, an instance migration to a member, whose daemon has not finished
	// starting yet, fails with.
	evacuationErr := errors.New(`Failed to migrate instance "amazonlinux-2" in project "default": websocket: bad handshake Daemon is starting up Daemon is starting up`)

	failures := 0

	success := driveRollingUpdate(t, ctx, clusterSvc, world, func(i int) bool {
		if failures >= 2 || len(serverClient.EvacuateCalls()) == 0 || world.pendingCount() == 0 {
			return false
		}

		world.releaseFailure(ctx, versionDataUpdated, evacuationErr)
		failures++

		return true
	})

	require.Equal(t, 2, failures)
	require.True(t, success, "the rolling update did not complete")

	// Both failures were retried, the third attempt succeeded.
	require.Len(t, serverClient.EvacuateCalls(), 3)
}

func TestClusterService_ClusterUpdateControlLoopRebootsWithoutNeedsRebootReported(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*100)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	serverClient := rollingUpdateServerClient(world)

	// The update is staged, so the server no longer asks for one, but it does not
	// ask for a reboot yet either.
	serverClient.UpdateOSFunc = func(ctx context.Context, server provisioning.Server) error {
		world.set(server.Name, versionDataUpdating, false)
		world.deferTransition(serverWorldTransition{
			server:      server.Name,
			versionData: versionDataUpdatedRebootNotReported,
		})

		return nil
	}

	env := setupControlLoopEnv(t, ctx, server)
	clusterSvc, _ := newControlLoopServices(t, env, t.Name(), serverClient, "2")

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	success := driveRollingUpdate(t, ctx, clusterSvc, world, nil)

	require.True(t, success, "the rolling update did not complete")

	// The reboot is owed from the moment the update is triggered, so it happens
	// even though the server never asked for it.
	require.Len(t, serverClient.UpdateOSCalls(), 1)
	require.Len(t, serverClient.RebootCalls(), 1)
}

func TestClusterService_ClusterUpdateControlLoopRetriggersStalledApplicationUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*100)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	serverClient := rollingUpdateServerClient(world)

	env := setupControlLoopEnv(t, ctx, server)
	clusterSvc, serverSvc := newControlLoopServices(t, env, t.Name(), serverClient, "2")

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	stalled := false

	success := driveRollingUpdate(t, ctx, clusterSvc, world, func(i int) bool {
		if stalled {
			return false
		}

		// An application update, whose completion is never reported, leaves the
		// server in the updating application state.
		err := serverSvc.UpdateSystemByName(ctx, "one", api.ServerUpdatePost{
			Applications: []api.ServerUpdateApplication{
				{
					Name:          "incus",
					TriggerUpdate: true,
				},
			},
		}, true)
		require.NoError(t, err)

		env.clock.advance(config.ClusterRollingUpdateApplyTimeout + time.Minute)
		stalled = true

		return true
	})

	require.True(t, stalled, "the application update was never triggered")
	require.True(t, success, "the rolling update did not complete")

	// The stalled application update was waited for and then superseded by the
	// update of the whole server, which covers the applications.
	require.Len(t, serverClient.UpdateApplicationCalls(), 1)
	require.Len(t, serverClient.UpdateOSCalls(), 1)
}
