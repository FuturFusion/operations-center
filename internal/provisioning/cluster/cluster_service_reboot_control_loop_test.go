package cluster_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	provisioningCluster "github.com/FuturFusion/operations-center/internal/provisioning/cluster"
	serviceMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

// rebootOnlyServer returns a server, which is fully up to date and does therefore
// not ask for a reboot on its own. An on demand rolling reboot has to reboot it
// nevertheless.
func rebootOnlyServer(t *testing.T, name string) provisioning.Server {
	t.Helper()

	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	return provisioning.Server{
		Name:          name,
		Cluster:       ptr.To("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://" + name + "/",
		Certificate:   string(certPEM),
		Fingerprint:   fingerprint,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: false,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   ptr.To(false),
			NeedsReboot:   ptr.To(false),
			InMaintenance: ptr.To(api.NotInMaintenance),
			UpdateChannel: "stable",
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}
}

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

	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	serverNames := make([]string, 0, len(servers))
	for _, server := range servers {
		serverNames = append(serverNames, server.Name)
	}

	clusterA := provisioning.Cluster{
		Name:          "clusterA",
		ConnectionURL: "https://cluster-one/",
		Certificate:   ptr.To(string(certPEM)),
		Fingerprint:   fingerprint,
		Status:        api.ClusterStatusReady,
		ServerNames:   serverNames,
		Channel:       "stable",
		Config: api.ClusterConfig{
			RollingRestart: api.ClusterConfigRollingRestart{
				PostRestoreDelay: (2 * controlLoopInterval).String(),
			},
		},
	}

	world := rebootOnlyWorld(servers...)

	logBuf := &bytes.Buffer{}
	var logSink io.Writer = logBuf
	if testing.Verbose() {
		logSink = io.MultiWriter(os.Stdout, logBuf)
	}

	err = logger.InitLogger(logSink, "", false, true, true)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	clusterDB := sqlite.NewCluster(tx)
	serverDB := sqlite.NewServer(tx)

	_, err = clusterDB.Create(ctx, clusterA)
	require.NoError(t, err)

	for _, server := range servers {
		_, err = serverDB.Create(ctx, server)
		require.NoError(t, err)
	}

	channelSvc := &serviceMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{}, nil
		},
	}

	// The most recent available version matches the installed one, nothing is
	// pending for any of the servers.
	updateSvc := &serviceMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{
				{
					ID:      1,
					UUID:    uuidgen.FromPattern(t, "1"),
					Version: "1",
					Files: provisioning.UpdateFiles{
						{Filename: "x86_64/IncusOS_20260610.img.gz"},
						{Filename: "x86_64/incus.raw.gz"},
					},
				},
			}, nil
		},
	}

	serverSvc := provisioningServer.New(
		serverDB, rebootOnlyServerClient(world), nil, nil, nil, channelSvc, updateSvc, tls.Certificate{},
		provisioningServer.WithRebootStatusUpdateGracePeriod(0),
	)

	clusterSvc := provisioningCluster.New(
		clusterDB, nil, nil, serverSvc, nil, nil, nil, nil,
		provisioningCluster.WithPendingUpdateRecheckInterval(controlLoopInterval),
		provisioningCluster.WithWarningEmitter(provisioning.LogWarningService{}),
	)

	serverSvc.SetClusterService(clusterSvc)

	// Trigger ClusterUpdateControlLoop also from server lifecycle events.
	lifecycle.ServerLifecycleSignal.AddListenerWithErr(func(ctx context.Context, slm lifecycle.ServerLifecycleMessage) error {
		err := clusterSvc.ClusterUpdateControlLoop(ctx, slm.Cluster)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to handle server lifecycle event", logger.Err(err), slog.String("server", slm.Server), slog.String("cluster", ptr.From(slm.Cluster)), slog.String("update_state", slm.ServerUpdateState.String()))
		}

		return err
	}, listenerName)
	t.Cleanup(func() {
		lifecycle.ServerLifecycleSignal.RemoveListener(listenerName)
	})

	return clusterSvc, world, logBuf
}

// driveRebootToCompletion runs the control loop until the rolling reboot has
// finished and returns the progress descriptions, a user would have observed.
func driveRebootToCompletion(t *testing.T, ctx context.Context, clusterSvc provisioning.ClusterService, world *serverWorld, iterations int) []string {
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

	clusterSvc, world, logBuf := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootCycleSingleNodeCluster", rebootOnlyServer(t, "one"))

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
		rebootOnlyServer(t, "serverA"),
		rebootOnlyServer(t, "serverB"),
		rebootOnlyServer(t, "serverC"),
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

	clusterSvc, world, _ := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootCycleMarksServerRebooted", rebootOnlyServer(t, "one"))

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

			server := rebootOnlyServer(t, "one")
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

	clusterSvc, _, _ := setupRebootOnlyCluster(t, ctx, "ClusterRollingRebootRejectsSecondRun", rebootOnlyServer(t, "one"))

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
