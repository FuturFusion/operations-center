package cluster_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

// stalledStepTimeout is short enough for the watchdog to fire within a test run,
// but far longer than a step of the test world needs, so only the deliberately
// stalled step ever reaches it.
var stalledStepTimeout = 30 * controlLoopInterval

// stalledRestoreServerClient drives the world through a rolling cluster update,
// except that the first restore behaves like one, whose Incus operation is gone:
// Incus keeps reporting the member as evacuated and the callback, Operations
// Center waits for, never fires.
func stalledRestoreServerClient(world *serverWorld, restoreCalls *atomic.Int32) *adapterMock.ServerClientPortMock {
	client := rollingUpdateServerClient(world)
	restore := client.RestoreFunc

	client.RestoreFunc = func(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
		if restoreCalls.Add(1) == 1 {
			// The member stays evacuated and nothing is left to complete the step.
			return nil
		}

		return restore(ctx, server, restoreModeSkip, callback)
	}

	return client
}

// driveUntilRunEnds runs the control loop until the rolling update has either
// completed or been parked in the error state, and returns the in progress
// status it ended with.
func driveUntilRunEnds(t *testing.T, ctx context.Context, clusterSvc provisioning.ClusterService, world *serverWorld, iterations int) api.ClusterUpdateInProgressStatus {
	t.Helper()

	for range iterations {
		cluster, err := clusterSvc.GetByName(ctx, "clusterA")
		require.NoError(t, err)

		inProgressStatus := cluster.UpdateStatus.InProgressStatus
		if inProgressStatus.InProgress == api.ClusterUpdateInProgressInactive ||
			inProgressStatus.InProgress == api.ClusterUpdateInProgressError {
			return inProgressStatus
		}

		pending := world.pendingCount()

		_ = clusterSvc.ClusterUpdateControlLoop(ctx, nil)

		if pending > 0 {
			world.release(ctx)
		}

		time.Sleep(controlLoopInterval)
	}

	t.Fatal("rolling update neither completed nor failed")

	return api.ClusterUpdateInProgressStatus{}
}

func TestClusterService_ClusterUpdateControlLoopRewindsStalledRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*200)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	var restoreCalls atomic.Int32

	rollingRestart := defaultRollingRestart
	rollingRestart.StepTimeout = stalledStepTimeout.String()

	clusterSvc, _, logBuf := setupControlLoopCluster(t, ctx, t.Name(), stalledRestoreServerClient(world, &restoreCalls), "2", rollingRestart, server)

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	inProgressStatus := driveUntilRunEnds(t, ctx, clusterSvc, world, 300)

	require.Equal(t, api.ClusterUpdateInProgressInactive, inProgressStatus.InProgress)
	require.Empty(t, inProgressStatus.Error)
	require.EqualValues(t, 2, restoreCalls.Load(), "the stalled restore has not been retried")
	require.Contains(t, logBuf.String(), "Resetting stalled maintenance state")
}

func TestClusterService_ClusterUpdateControlLoopReportsStalledEvacuation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*200)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	client := rollingUpdateServerClient(world)
	client.EvacuateFunc = func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
		// Incus starts the evacuation, but it never completes and the callback, which
		// Operations Center waits for, never fires.
		world.set(server.Name, versionDataEvacuating, false)

		return nil
	}

	rollingRestart := defaultRollingRestart
	rollingRestart.StepTimeout = stalledStepTimeout.String()

	clusterSvc, _, _ := setupControlLoopCluster(t, ctx, t.Name(), client, "2", rollingRestart, server)

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	inProgressStatus := driveUntilRunEnds(t, ctx, clusterSvc, world, 300)

	require.Equal(t, api.ClusterUpdateInProgressError, inProgressStatus.InProgress)
	require.Contains(t, inProgressStatus.Error, `server "one"`)
	require.Contains(t, inProgressStatus.Error, `did not leave state "evacuating"`)
}

func TestClusterService_ClusterUpdateControlLoopKeepsWaitingWithinStepTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*200)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	clusterSvc, _, logBuf := setupControlLoopCluster(t, ctx, t.Name(), rollingUpdateServerClient(world), "2", defaultRollingRestart, server)

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	observed := driveRollingRestartToCompletion(t, ctx, clusterSvc, world, 300)

	require.NotContains(t, logBuf.String(), "Resetting stalled maintenance state")
	require.NotContains(t, logBuf.String(), "did not leave state")
	requireProgressOnlyMovesForward(t, observed)
	require.Contains(t, observed, `[4/9] evacuating server "one"`)
	require.Contains(t, observed, `[8/9] restoring server "one"`)
}
