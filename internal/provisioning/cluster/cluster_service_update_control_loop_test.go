package cluster_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"os"
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
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	asyncActionsDelay   = 100 * time.Millisecond
	controlLoopInterval = 10 * time.Millisecond
)

func TestClusterService_ClusterUpdateControlLoopSingleNodeCluster(t *testing.T) {
	// Test data
	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	clusterA := provisioning.Cluster{
		Name:          "clusterA",
		ConnectionURL: "https://cluster-one/",
		Certificate:   new(string(certPEM)),
		Fingerprint:   fingerprint,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"serverA"},
		Channel:       "stable",
		Config: api.ClusterConfig{
			RollingRestart: api.ClusterConfigRollingRestart{
				PostRestoreDelay: (2 * controlLoopInterval).String(),
			},
		},
	}

	serverA := provisioning.Server{
		Name:          "one",
		Cluster:       new("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		Certificate:   string(certPEM),
		Fingerprint:   fingerprint,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   new(true),
			InMaintenance: new(api.NotInMaintenance),
			UpdateChannel: "stable",
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	world := newServerWorld(map[string]api.ServerVersionData{
		"one": {
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
			UpdateChannel: "stable",
		},
	})

	// Setup
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*50)
	defer cancel()

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
		err = db.Close()
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
	_, err = serverDB.Create(ctx, serverA)
	require.NoError(t, err)

	channelSvc := &serviceMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{}, nil
		},
	}

	updateSvc := &serviceMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			}, nil
		},
	}

	serverClient := &adapterMock.ServerClientPortMock{
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
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
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
			world.set(server.Name, versionDataUpdating, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataUpdated,
			})

			return nil
		},
		EvacuateFunc: func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
			world.set(server.Name, versionDataEvacuating, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataEvacuated,
				callback:    callback,
			})

			return nil
		},
		RebootFunc: func(ctx context.Context, server provisioning.Server) error {
			world.set(server.Name, versionDataRebooting, true)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRebooted,
			})

			return nil
		},
		RestoreFunc: func(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
			world.set(server.Name, versionDataRestoring, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRestored,
				callback:    callback,
			})

			return nil
		},
	}

	serverSvc := provisioningServer.New(
		serverDB, serverClient, nil, nil, nil, channelSvc, updateSvc, tls.Certificate{},
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
	}, "ClusterUpdateCycleSingleNodeCluster")
	t.Cleanup(func() {
		lifecycle.ServerLifecycleSignal.RemoveListener("ClusterUpdateCycleSingleNodeCluster")
	})

	// Run test
	err = clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
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
}

func TestClusterService_ClusterUpdateControlLoopMultiNodeCluster(t *testing.T) {
	// Test data
	certPEMA, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	certPEMB, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	certPEMC, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprintA, err := incustls.CertFingerprintStr(string(certPEMA))
	require.NoError(t, err)

	fingerprintB, err := incustls.CertFingerprintStr(string(certPEMB))
	require.NoError(t, err)

	fingerprintC, err := incustls.CertFingerprintStr(string(certPEMC))
	require.NoError(t, err)

	clusterA := provisioning.Cluster{
		Name:          "clusterA",
		ConnectionURL: "https://cluster-one/",
		Certificate:   new(string(certPEMA)),
		Fingerprint:   fingerprintA,
		Status:        api.ClusterStatusReady,
		ServerNames:   []string{"serverA", "serverB", "serverC"},
		Channel:       "stable",
		Config: api.ClusterConfig{
			RollingRestart: api.ClusterConfigRollingRestart{
				PostRestoreDelay: (2 * controlLoopInterval).String(),
			},
		},
	}

	serverA := provisioning.Server{
		Name:          "serverA",
		Cluster:       new("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://serverA/",
		Certificate:   string(certPEMA),
		Fingerprint:   fingerprintA,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   new(true),
			InMaintenance: new(api.NotInMaintenance),
			UpdateChannel: "stable",
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	serverB := provisioning.Server{
		Name:          "serverB",
		Cluster:       new("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://serverB/",
		Certificate:   string(certPEMB),
		Fingerprint:   fingerprintB,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   new(true),
			InMaintenance: new(api.NotInMaintenance),
			UpdateChannel: "stable",
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	serverC := provisioning.Server{
		Name:          "serverC",
		Cluster:       new("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://serverC/",
		Certificate:   string(certPEMC),
		Fingerprint:   fingerprintC,
		HardwareData:  api.HardwareData{},
		VersionData: api.ServerVersionData{
			OS: api.OSVersionData{
				Name:        "incusos",
				Version:     "1",
				VersionNext: "1",
				NeedsReboot: true,
			},
			Applications: []api.ApplicationVersionData{
				{
					Name:          "incus",
					Version:       "1",
					InMaintenance: api.NotInMaintenance,
				},
			},
			NeedsUpdate:   new(true),
			InMaintenance: new(api.NotInMaintenance),
			UpdateChannel: "stable",
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}

	world := newServerWorld(map[string]api.ServerVersionData{
		"serverA": versionDataInitial,
		"serverB": versionDataInitial,
		"serverC": versionDataInitial,
	})

	// Setup
	ctx, cancel := context.WithTimeout(t.Context(), asyncActionsDelay*75)
	defer cancel()

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
		err = db.Close()
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
	_, err = serverDB.Create(ctx, serverA)
	require.NoError(t, err)
	_, err = serverDB.Create(ctx, serverB)
	require.NoError(t, err)
	_, err = serverDB.Create(ctx, serverC)
	require.NoError(t, err)

	channelSvc := &serviceMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{}, nil
		},
	}

	updateSvc := &serviceMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{
				{
					ID:      2,
					UUID:    uuidgen.FromPattern(t, "2"),
					Version: "2",
					Files: provisioning.UpdateFiles{
						{
							Filename: "x86_64/IncusOS_20260610.img.gz",
						},
						{
							Filename: "x86_64/incus.raw.gz",
						},
					},
				},
			}, nil
		},
	}

	serverClient := &adapterMock.ServerClientPortMock{
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
								Addresses: []string{
									"192.168.0.100",
								},
								Roles: []string{
									"management",
								},
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
			world.set(server.Name, versionDataUpdating, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataUpdated,
			})

			return nil
		},
		EvacuateFunc: func(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
			world.set(server.Name, versionDataEvacuating, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataEvacuated,
				callback:    callback,
			})

			return nil
		},
		RebootFunc: func(ctx context.Context, server provisioning.Server) error {
			world.set(server.Name, versionDataRebooting, true)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRebooted,
			})

			return nil
		},
		RestoreFunc: func(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
			world.set(server.Name, versionDataRestoring, false)
			world.deferTransition(serverWorldTransition{
				server:      server.Name,
				versionData: versionDataRestored,
				callback:    callback,
			})

			return nil
		},
	}

	serverSvc := provisioningServer.New(
		serverDB, serverClient, nil, nil, nil, channelSvc, updateSvc, tls.Certificate{},
		provisioningServer.WithRebootStatusUpdateGracePeriod(0),
	)

	clusterSvc := provisioningCluster.New(
		clusterDB, nil, nil, serverSvc, nil, nil, nil, nil,
		provisioningCluster.WithPendingUpdateRecheckInterval(controlLoopInterval),
	)

	serverSvc.SetClusterService(clusterSvc)

	// Trigger ClusterUpdateControlLoop also from server lifecycle events.
	lifecycle.ServerLifecycleSignal.AddListenerWithErr(func(ctx context.Context, slm lifecycle.ServerLifecycleMessage) error {
		err := clusterSvc.ClusterUpdateControlLoop(ctx, slm.Cluster)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to handle server lifecycle event", logger.Err(err), slog.String("server", slm.Server), slog.String("cluster", ptr.From(slm.Cluster)), slog.String("update_state", slm.ServerUpdateState.String()))
		}

		return err
	}, "ClusterUpdateCycleMultiNodeCluster")
	t.Cleanup(func() {
		lifecycle.ServerLifecycleSignal.RemoveListener("ClusterUpdateCycleMultiNodeCluster")
	})

	// Run test
	err = clusterSvc.LaunchClusterUpdate(ctx, "clusterA", true)
	require.NoError(t, err)

	var observed []string

	success := false
	for range 200 {
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

	require.True(t, success)

	requireProgressOnlyMovesForward(t, observed)

	require.Equal(t, []string{
		`[ 1/27] update pending server "serverA"`,
		`[ 2/27] updating server "serverA"`,
		`[ 3/27] update pending server "serverB"`,
		`[ 4/27] updating server "serverB"`,
		`[ 5/27] update pending server "serverC"`,
		`[ 6/27] updating server "serverC"`,
		`[ 7/27] evacuation pending server "serverA"`,
		`[ 8/27] evacuating server "serverA"`,
		`[ 9/27] in maintenance, reboot pending server "serverA"`,
		`[10/27] in maintenance, rebooting server "serverA"`,
		`[11/27] in maintenance, restore pending server "serverA"`,
		`[12/27] restoring server "serverA"`,
		`[13/27] post restore server "serverA"`,
		`[14/27] evacuation pending server "serverB"`,
		`[15/27] evacuating server "serverB"`,
		`[16/27] in maintenance, reboot pending server "serverB"`,
		`[17/27] in maintenance, rebooting server "serverB"`,
		`[18/27] in maintenance, restore pending server "serverB"`,
		`[19/27] restoring server "serverB"`,
		`[20/27] post restore server "serverB"`,
		`[21/27] evacuation pending server "serverC"`,
		`[22/27] evacuating server "serverC"`,
		`[23/27] in maintenance, reboot pending server "serverC"`,
		`[24/27] in maintenance, rebooting server "serverC"`,
		`[25/27] in maintenance, restore pending server "serverC"`,
		`[26/27] restoring server "serverC"`,
		`[27/27] post restore server "serverC"`,
	}, clusterUpdateStatesFromLog(t, logBuf.String()))
}

func TestClusterService_ClusterUpdateControlLoop(t *testing.T) {
	tests := []struct {
		name                           string
		repoGetAll                     []queue.Item[provisioning.Clusters]
		repoGetByNameErr               error
		repoUpdateErr                  error
		serverSvcPollServersErr        error
		serverSvcGetAllWithFilter      []queue.Item[provisioning.Servers]
		serverSvcUpdateByNameErrs      queue.Errs
		serverSvcRebootSystemByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - no clusters with in progress update",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - executeRollingRestartNextStep - 1st and 3rd server manually evacuated before",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress:      api.ClusterUpdateInProgressRollingRestart,
									EvacuatedBefore: []string{"server1", "server3"},
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server1",
							ConnectionURL: "https://server1:8443",
							Cluster:       new("cluster"),
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(false),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
						{
							Name:          "server2",
							ConnectionURL: "https://server1:8443",
							Cluster:       new("cluster"),
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(true),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
						{
							Name:          "server3",
							ConnectionURL: "https://server2:8443",
							Cluster:       new("cluster"),
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(true),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - GetAllWithFilter",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - cluster has in progress error",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
									Error:      "error",
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - serverSvc.PollServers",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
			},
			serverSvcPollServersErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - serverSvc.GetAllWithFilter",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - cluster without servers",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - server in undefined state",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusUnknown, // server state undefined
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server update state for "server" (https://server:8443) is undefined`)
			},
		},

		{
			name: "error - executeRollingUpdate - serverSvc.UpdateSystemByName",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:   "server",
							Status: api.ServerStatusReady,
							VersionData: api.ServerVersionData{
								NeedsUpdate: new(true),
							},
						},
					},
				},
			},
			serverSvcUpdateByNameErrs: queue.Errs{
				boom.Error,
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - executeRollingUpdate - repo.GetByName",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:   "server",
							Status: api.ServerStatusReady,
						},
					},
				},
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - executeRollingUpdate - repo.Update",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:   "server",
							Status: api.ServerStatusReady,
						},
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},

		{
			name: "error - executeRollingRestartNextStep - server in updating state",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "server",
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailReadyUpdatingOS,
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server "server" is updating while a cluster wide rolling reboot cycle is ongoing`)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - server update state not supported",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusOffline,
							StatusDetail:  api.ServerStatusDetailOfflineRebooting,
							VersionData:   api.ServerVersionData{},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server update state "rebooting" for "server" (https://server:8443) is not supported`)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - 2nd server undefined state",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "server",
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(false),
								InMaintenance: new(api.InMaintenanceEvacuated),
							},
						},
						{
							Name:          "server",
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusUnknown, // server state undefined
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Rolling update blocked, server "server" (https://server:8443) is in unknown state`)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - 2nd server updating",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "server1",
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(false),
								InMaintenance: new(api.InMaintenanceEvacuated),
							},
						},
						{
							Name:         "server2",
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailReadyUpdatingOS,
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server "server2" is updating while a cluster wide rolling reboot cycle is ongoing`)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - 2nd server evacuated",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:         "server1",
							Status:       api.ServerStatusReady,
							StatusDetail: api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(false),
								InMaintenance: new(api.InMaintenanceEvacuated),
							},
						},
						{
							Name:          "server2",
							ConnectionURL: "https://server2:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(false),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Rolling update blocked, out of order update for server "server2" (https://server2:8443) is ongoing, state in maintenance, restore pending`)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - nextAction - retryable",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							Cluster:       new("cluster"),
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(true),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
					},
				},
			},
			serverSvcRebootSystemByNameErr: domain.NewRetryableErr(boom.Error),

			assertErr: require.NoError,
		},
		{
			name: "error - executeRollingRestartNextStep - nextAction - terminal error",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							Cluster:       new("cluster"),
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(true),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
					},
				},
			},
			serverSvcRebootSystemByNameErr: errors.Join(boom.Error, domain.ErrTerminal),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrTerminal)
				boom.ErrorIs(tt, err)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - nextAction - terminal error - failed update",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							Cluster:       new("cluster"),
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(true),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
					},
				},
			},
			serverSvcRebootSystemByNameErr: domain.ErrTerminal,
			repoUpdateErr:                  boom.Error,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrTerminal)
				boom.ErrorIs(tt, err)
			},
		},
		{
			name: "error - executeRollingRestartNextStep - nextAction",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							Cluster:       new("cluster"),
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData: api.ServerVersionData{
								NeedsUpdate:   new(false),
								NeedsReboot:   new(true),
								InMaintenance: new(api.InMaintenanceEvacuated),
								Applications: []api.ApplicationVersionData{
									{
										Name: "incus",
									},
								},
							},
						},
					},
				},
			},
			serverSvcRebootSystemByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - executeRollingRestartNextStep - update done - repo.GetByName",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							Cluster:       new("cluster"),
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData:   api.ServerVersionData{},
						},
					},
				},
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - executeRollingRestartNextStep - update done - repo.Update",
			repoGetAll: []queue.Item[provisioning.Clusters]{
				{
					Value: provisioning.Clusters{
						{
							Name: "one",
							UpdateStatus: api.ClusterUpdateStatus{
								InProgressStatus: api.ClusterUpdateInProgressStatus{
									InProgress: api.ClusterUpdateInProgressRollingRestart,
								},
							},
						},
					},
				},
			},
			serverSvcGetAllWithFilter: []queue.Item[provisioning.Servers]{
				// cluster GetAllWithFilter
				{},
				// GetAllWithFilter
				{
					Value: provisioning.Servers{
						{
							Name:          "server",
							Cluster:       new("cluster"),
							ConnectionURL: "https://server:8443",
							Status:        api.ServerStatusReady,
							StatusDetail:  api.ServerStatusDetailNone,
							VersionData:   api.ServerVersionData{},
						},
					},
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Clusters, error) {
					return queue.Pop(t, &tc.repoGetAll)
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					return &provisioning.Cluster{}, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					return tc.repoUpdateErr
				},
			}

			serverSvc := &serviceMock.ServerServiceMock{
				PollServersFunc: func(ctx context.Context, serverFilter provisioning.ServerFilter, updateServerConfiguration bool) error {
					return tc.serverSvcPollServersErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return queue.Pop(t, &tc.serverSvcGetAllWithFilter)
				},
				UpdateSystemByNameFunc: func(ctx context.Context, name string, updateRequest api.ServerUpdatePost, force bool) error {
					return tc.serverSvcUpdateByNameErrs.Pop(t)
				},
				RebootSystemByNameFunc: func(ctx context.Context, name string, force bool) error {
					return tc.serverSvcRebootSystemByNameErr
				},
				RestoreSystemByNameFunc: func(ctx context.Context, name string, clusterUpdate bool, force bool, restoreModeSkip bool) error {
					return nil
				},
			}

			clusterSvc := provisioningCluster.New(repo, nil, nil, serverSvc, nil, nil, nil, nil)

			// Run test
			err := clusterSvc.ClusterUpdateControlLoop(t.Context(), nil)

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoGetAll)
			require.Empty(t, tc.serverSvcGetAllWithFilter)
			require.Empty(t, tc.serverSvcUpdateByNameErrs)
		})
	}
}
