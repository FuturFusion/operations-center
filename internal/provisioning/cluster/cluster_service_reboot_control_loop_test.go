package cluster_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

// rebootOnlyServerClient returns a client mock, which drives the given world
// through an on demand rolling reboot, i.e. without ever applying an update.
func rebootOnlyServerClient(world *serverWorld) *adapterMock.ServerClientPortMock {
	return &adapterMock.ServerClientPortMock{
		UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemUpdate) error {
			return nil
		},
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			if world.isRebooting(endpoint.GetName()) {
				return domain.NewRetryableErr(errors.New("rebooting"))
			}

			return nil
		},
		IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
			return nil
		},
		GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
			return api.HardwareData{}, nil
		},
		GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
			return api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
							"eth0": {
								Addresses: []string{"192.168.0.100"},
								Roles:     []string{"management"},
							},
						},
					},
				},
			}, nil
		},
		GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
			return world.getVersionData(server.Name), nil
		},
		GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
			return api.ServerTypeIncus, nil
		},
		UpdateOSFunc: func(ctx context.Context, server provisioning.Server) error {
			return errors.New("no update must be triggered during an on demand rolling reboot")
		},
		EvacuateFunc: func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
			world.set(server.Name, versionDataRebootOnlyEvacuating, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRebootOnlyEvacuated,
				callback:    callback,
			})

			return nil
		},
		RebootFunc: func(ctx context.Context, server provisioning.Server) error {
			world.set(server.Name, versionDataRebootOnlyEvacuated, true)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRebootOnlyEvacuated,
			})

			return nil
		},
		RestoreFunc: func(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
			world.set(server.Name, versionDataRebootOnlyRestoring, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRebootOnlyRestored,
				callback:    callback,
			})

			return nil
		},
	}
}

// rebootOnlyWorld returns a world, which reports what the given servers have been
// registered with, minus the fields, that are calculated during polling.
func rebootOnlyWorld(servers ...provisioning.Server) *serverWorld {
	versionData := make(map[string]api.ServerVersionData, len(servers))
	for _, server := range servers {
		versionData[server.Name] = api.ServerVersionData{
			OS:            server.VersionData.OS,
			Applications:  slices.Clone(server.VersionData.Applications),
			UpdateChannel: server.VersionData.UpdateChannel,
		}
	}

	return newServerWorld(versionData)
}

// setupRebootOnlyCluster wires up a cluster service backed by a real SQLite
// schema, with the given servers already registered and up to date.
func setupRebootOnlyCluster(t *testing.T, ctx context.Context, listenerName string, servers ...provisioning.Server) (provisioning.ClusterService, *serverWorld, *bytes.Buffer) {
	t.Helper()

	world := rebootOnlyWorld(servers...)

	// The most recent available version matches the installed one, nothing is
	// pending for any of the servers.
	clusterSvc, _, logBuf := setupControlLoopCluster(t, ctx, listenerName, rebootOnlyServerClient(world), "1", servers...)

	return clusterSvc, world, logBuf
}

// driveRebootToCompletion runs the control loop until the rolling reboot has
// finished and returns the progress descriptions, a user would have observed.
func driveRebootToCompletion(t *testing.T, ctx context.Context, clusterSvc provisioning.ClusterService, world *serverWorld, iterations int, beforeIteration ...func(ctx context.Context)) []string {
	t.Helper()

	var observed []string

	success := false
	for range iterations {
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

		for _, hook := range beforeIteration {
			hook(ctx)
		}

		err = clusterSvc.ClusterUpdateControlLoop(ctx, nil)
		if !domain.IsRetryableError(err) {
			require.NoError(t, err)
		}

		if pending > 0 {
			world.release(ctx)
		}

		time.Sleep(controlLoopInterval)
	}

	require.True(t, success, "rolling reboot did not complete, observed: %v", observed)

	return observed
}

func TestClusterService_ClusterRollingRebootControlLoopSingleNodeCluster(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	clusterSvc, world, logBuf := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootCycleSingleNodeCluster", clusterMemberServer(t, "one"))

	err := clusterSvc.LaunchClusterReboot(ctx, "clusterA")
	require.NoError(t, err)

	c, err := clusterSvc.GetByName(ctx, "clusterA")
	require.NoError(t, err)
	require.Equal(t, api.ClusterUpdateInProgressRollingReboot, c.UpdateStatus.InProgressStatus.InProgress)
	require.Equal(t, []string{"one"}, c.UpdateStatus.InProgressStatus.PendingReboot)

	observed := driveRebootToCompletion(t, ctx, clusterSvc, world, 100)

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
}

func TestClusterService_ClusterRollingRebootControlLoopMultiNodeCluster(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*100)
	defer cancel()

	clusterSvc, world, logBuf := setupRebootOnlyCluster(
		t, ctx, "ClusterRollingRebootCycleMultiNodeCluster",
		clusterMemberServer(t, "serverA"),
		clusterMemberServer(t, "serverB"),
		clusterMemberServer(t, "serverC"),
	)

	err := clusterSvc.LaunchClusterReboot(ctx, "clusterA")
	require.NoError(t, err)

	c, err := clusterSvc.GetByName(ctx, "clusterA")
	require.NoError(t, err)
	require.Equal(t, []string{"serverA", "serverB", "serverC"}, c.UpdateStatus.InProgressStatus.PendingReboot)

	observed := driveRebootToCompletion(t, ctx, clusterSvc, world, 200)

	requireProgressOnlyMovesForward(t, observed)

	require.Equal(t, []string{
		`[ 1/21] evacuation pending server "serverA"`,
		`[ 2/21] evacuating server "serverA"`,
		`[ 3/21] in maintenance, reboot pending server "serverA"`,
		`[ 4/21] in maintenance, rebooting server "serverA"`,
		`[ 5/21] in maintenance, restore pending server "serverA"`,
		`[ 6/21] restoring server "serverA"`,
		`[ 7/21] post restore server "serverA"`,
		`[ 8/21] evacuation pending server "serverB"`,
		`[ 9/21] evacuating server "serverB"`,
		`[10/21] in maintenance, reboot pending server "serverB"`,
		`[11/21] in maintenance, rebooting server "serverB"`,
		`[12/21] in maintenance, restore pending server "serverB"`,
		`[13/21] restoring server "serverB"`,
		`[14/21] post restore server "serverB"`,
		`[15/21] evacuation pending server "serverC"`,
		`[16/21] evacuating server "serverC"`,
		`[17/21] in maintenance, reboot pending server "serverC"`,
		`[18/21] in maintenance, rebooting server "serverC"`,
		`[19/21] in maintenance, restore pending server "serverC"`,
		`[20/21] restoring server "serverC"`,
		`[21/21] post restore server "serverC"`,
	}, clusterUpdateStatesFromLog(t, logBuf.String()))
}

func TestClusterService_ClusterRollingRebootControlLoopMarksServerRebooted(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	clusterSvc, world, _ := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootCycleMarksServerRebooted", clusterMemberServer(t, "one"))

	err := clusterSvc.LaunchClusterReboot(ctx, "clusterA")
	require.NoError(t, err)

	// Drive the loop until the reboot of the single server has been triggered.
	rebooted := false
	for range 100 {
		c, err := clusterSvc.GetByName(ctx, "clusterA")
		require.NoError(t, err)
		require.Empty(t, c.UpdateStatus.InProgressStatus.Error)

		if !slices.Contains(c.UpdateStatus.InProgressStatus.PendingReboot, "one") {
			rebooted = true
			break
		}

		pending := world.pendingCount()

		err = clusterSvc.ClusterUpdateControlLoop(ctx, nil)
		if !domain.IsRetryableError(err) {
			require.NoError(t, err)
		}

		if pending > 0 {
			world.release(ctx)
		}

		time.Sleep(controlLoopInterval)
	}

	require.True(t, rebooted, "server was not dropped from the pending reboot list")

	c, err := clusterSvc.GetByName(ctx, "clusterA")
	require.NoError(t, err)
	require.Empty(t, c.UpdateStatus.InProgressStatus.PendingReboot)
	require.Equal(t, api.ClusterUpdateInProgressRollingReboot, c.UpdateStatus.InProgressStatus.InProgress)
}

func TestClusterService_LaunchClusterRebootRejectsUnsuitableServers(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(server *provisioning.Server)
		wantErrIs string
	}{
		{
			name: "server is applying an application update",
			mutate: func(server *provisioning.Server) {
				server.StatusDetail = api.ServerStatusDetailReadyUpdatingApplication
			},
			wantErrIs: "is busy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
			defer cancel()

			server := clusterMemberServer(t, "one")
			tc.mutate(&server)

			clusterSvc, _, _ := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootReject"+tc.name, server)

			err := clusterSvc.LaunchClusterReboot(ctx, "clusterA")
			var verr domain.ErrValidation
			require.ErrorAs(t, err, &verr)
			require.ErrorContains(t, err, tc.wantErrIs)

			c, err := clusterSvc.GetByName(ctx, "clusterA")
			require.NoError(t, err)
			require.Equal(t, api.ClusterUpdateInProgressInactive, c.UpdateStatus.InProgressStatus.InProgress)
			require.Empty(t, c.UpdateStatus.InProgressStatus.PendingReboot)
		})
	}
}

func TestClusterService_LaunchClusterRebootRejectsSecondRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

	clusterSvc, _, _ := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootRejectsSecondRun", clusterMemberServer(t, "one"))

	err := clusterSvc.LaunchClusterReboot(ctx, "clusterA")
	require.NoError(t, err)

	err = clusterSvc.LaunchClusterReboot(ctx, "clusterA")
	require.ErrorIs(t, err, domain.ErrOperationNotPermitted)

	err = clusterSvc.AbortClusterOperation(ctx, "clusterA")
	require.NoError(t, err)

	c, err := clusterSvc.GetByName(ctx, "clusterA")
	require.NoError(t, err)
	require.Equal(t, api.ClusterUpdateInProgressInactive, c.UpdateStatus.InProgressStatus.InProgress)
	require.Empty(t, c.UpdateStatus.InProgressStatus.PendingReboot)
}
