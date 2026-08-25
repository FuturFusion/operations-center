package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

// daemonBackgroundPolling returns a function, which performs the polling of the
// servers in a transient state the same way, the background tasks of the daemon
// do it (see Daemon.setupBackgroundTasks). These tasks run independently of the
// cluster update control loop and therefore concurrently to a rolling cluster
// update or reboot.
func daemonBackgroundPolling(t *testing.T, serverSvc provisioning.ServerService) func(ctx context.Context) {
	t.Helper()

	tasks := []struct {
		filter                    provisioning.ServerFilter
		updateServerConfiguration bool
	}{
		{
			filter: provisioning.ServerFilter{
				Status: new(api.ServerStatusPending),
			},
			updateServerConfiguration: true,
		},
		{
			filter: provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyUpdatingOS),
			},
			updateServerConfiguration: true,
		},
		{
			filter: provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyEvacuating),
			},
			updateServerConfiguration: false,
		},
		{
			filter: provisioning.ServerFilter{
				Status:       new(api.ServerStatusReady),
				StatusDetail: new(api.ServerStatusDetailReadyRestoring),
			},
			updateServerConfiguration: false,
		},
		{
			filter: provisioning.ServerFilter{
				Status:       new(api.ServerStatusOffline),
				StatusDetail: new(api.ServerStatusDetailOfflineRebooting),
			},
			updateServerConfiguration: true,
		},
		{
			filter: provisioning.ServerFilter{
				Status:       new(api.ServerStatusOffline),
				StatusDetail: new(api.ServerStatusDetailOfflineUnresponsive),
			},
			updateServerConfiguration: true,
		},
	}

	return func(ctx context.Context) {
		for _, task := range tasks {
			err := serverSvc.PollServers(ctx, task.filter, task.updateServerConfiguration)
			if err != nil && !domain.IsRetryableError(err) {
				require.NoError(t, err)
			}
		}
	}
}

// A rolling cluster update passes through the very same states and triggers every
// action exactly once, while the background polling of the daemon refreshes the
// state of the servers in a transient state in parallel.
func TestClusterService_ClusterUpdateControlLoopWithBackgroundPolling(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": versionDataInitial,
	})

	serverClient := rollingUpdateServerClient(world)

	clusterSvc, serverSvc, logBuf := setupControlLoopCluster(t, ctx, "ClusterUpdateCycleWithBackgroundPolling", serverClient, "2", server)

	backgroundPolling := daemonBackgroundPolling(t, serverSvc)

	err := clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	var observed []string

	success := false
	for range 100 {
		c, err := clusterSvc.GetByName(ctx, "clusterA")
		require.NoError(t, err)
		if c.UpdateStatus.InProgressStatus.InProgress == api.ClusterUpdateInProgressInactive {
			success = true
			break
		}

		require.Empty(t, c.UpdateStatus.InProgressStatus.Error)

		// Observe the progress the same way a user does.
		observed = append(observed, ptr.From(c.UpdateStatus.InProgressStatus.StatusDescription))

		// A server completes an action, which has been triggered on it, asynchronously.
		// Let it complete only if it was already running before this iteration, so that
		// the state, the action leads to, is reported at least once.
		pending := world.pendingCount()

		backgroundPolling(ctx)

		err = clusterSvc.ClusterUpdateControlLoop(ctx, nil)
		if !domain.IsRetryableError(err) {
			require.NoError(t, err)
		}

		if pending > 0 {
			world.release(ctx)
		}

		time.Sleep(controlLoopInterval)
	}

	require.True(t, success)

	requireProgressOnlyMovesForward(t, observed)

	require.Equal(t, []string{
		`[1/9] update pending server "one"`,
		`[2/9] updating server "one"`,
		`[3/9] evacuation pending server "one"`,
		`[4/9] evacuating server "one"`,
		`[5/9] in maintenance, reboot pending server "one"`,
		`[6/9] in maintenance, rebooting server "one"`,
		`[7/9] in maintenance, restore pending server "one"`,
		`[8/9] restoring server "one"`,
		`[9/9] post restore server "one"`,
	}, clusterUpdateStatesFromLog(t, logBuf.String()))

	// The background polling must not cause any action to be triggered a second
	// time, e.g. because it reports state from before the action.
	require.Len(t, serverClient.UpdateOSCalls(), 1)
	require.Len(t, serverClient.EvacuateCalls(), 1)
	require.Len(t, serverClient.RebootCalls(), 1)
	require.Len(t, serverClient.RestoreCalls(), 1)

	// The server is reported as up to date, the state, the server reports after the
	// update, has been picked up.
	updatedServer, err := serverSvc.GetByName(ctx, "one")
	require.NoError(t, err)
	require.Equal(t, api.ServerUpdateStateUpToDate, updatedServer.UpdateState())
}

// An on demand rolling reboot passes through the very same states and triggers
// every action exactly once, while the background polling of the daemon refreshes
// the state of the servers in a transient state in parallel.
func TestClusterService_ClusterRollingRebootControlLoopWithBackgroundPolling(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	server := clusterMemberServer(t, "one")

	world := rebootOnlyWorld(server)
	serverClient := rebootOnlyServerClient(world)

	clusterSvc, serverSvc, logBuf := setupControlLoopCluster(t, ctx, "ClusterRollingRebootCycleWithBackgroundPolling", serverClient, "1", server)

	err := clusterSvc.LaunchClusterReboot(ctx, "clusterA")
	require.NoError(t, err)

	observed := driveRebootToCompletion(t, ctx, clusterSvc, world, 100, daemonBackgroundPolling(t, serverSvc))

	requireProgressOnlyMovesForward(t, observed)

	require.Equal(t, []string{
		`[1/7] evacuation pending server "one"`,
		`[2/7] evacuating server "one"`,
		`[3/7] in maintenance, reboot pending server "one"`,
		`[4/7] in maintenance, rebooting server "one"`,
		`[5/7] in maintenance, restore pending server "one"`,
		`[6/7] restoring server "one"`,
		`[7/7] post restore server "one"`,
	}, clusterUpdateStatesFromLog(t, logBuf.String()))

	// The background polling must not cause any action to be triggered a second
	// time, in particular the reboot, which is synthesized from the list of servers
	// with a pending reboot.
	require.Len(t, serverClient.EvacuateCalls(), 1)
	require.Len(t, serverClient.RebootCalls(), 1)
	require.Len(t, serverClient.RestoreCalls(), 1)
}
