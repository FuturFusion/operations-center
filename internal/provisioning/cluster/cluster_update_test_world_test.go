package cluster_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"

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

// clusterMemberServer returns a server, which is a member of the cluster and
// which is fully up to date, so it neither asks for an update nor for a reboot on
// its own.
func clusterMemberServer(t *testing.T, name string) provisioning.Server {
	t.Helper()

	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	fingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	return provisioning.Server{
		Name:          name,
		Cluster:       new("clusterA"),
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://" + name + "/",
		Certificate:   new(string(certPEM)),
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
			NeedsUpdate:   new(false),
			NeedsReboot:   new(false),
			InMaintenance: new(api.NotInMaintenance),
			UpdateChannel: "stable",
		},
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
	}
}

// setupControlLoopCluster wires up a cluster service backed by a real SQLite
// schema, with the given servers already registered. The fake servers are served
// by the given client mock and availableVersion is the most recent update
// version, which is available in the update channel of the cluster.
func setupControlLoopCluster(t *testing.T, ctx context.Context, listenerName string, serverClient *adapterMock.ServerClientPortMock, availableVersion string, servers ...provisioning.Server) (provisioning.ClusterService, provisioning.ServerService, *bytes.Buffer) {
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
		Certificate:   new(string(certPEM)),
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

	updateSvc := &serviceMock.UpdateServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
			return provisioning.Updates{
				{
					ID:      1,
					UUID:    uuidgen.FromPattern(t, availableVersion),
					Version: availableVersion,
					Files: provisioning.UpdateFiles{
						{Filename: "x86_64/IncusOS_20260610.img.gz"},
						{Filename: "x86_64/incus.raw.gz"},
					},
				},
			}, nil
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
	}, listenerName)
	t.Cleanup(func() {
		lifecycle.ServerLifecycleSignal.RemoveListener(listenerName)
	})

	return clusterSvc, serverSvc, logBuf
}

// The version data, the fake servers report while passing through a rolling
// cluster update. Version "1" is installed initially, version "2" is available.
func versionData(osVersion string, osVersionNext string, needsReboot bool, incusVersion string, inMaintenance api.InMaintenanceState) api.ServerVersionData {
	return api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "incusos",
			Version:     osVersion,
			VersionNext: osVersionNext,
			NeedsReboot: needsReboot,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:          "incus",
				Version:       incusVersion,
				InMaintenance: inMaintenance,
			},
		},
		UpdateChannel: "stable",
	}
}

var (
	versionDataInitial    = versionData("1", "1", false, "1", api.NotInMaintenance)
	versionDataUpdating   = versionData("1", "1", false, "1", api.NotInMaintenance)
	versionDataUpdated    = versionData("1", "2", true, "2", api.NotInMaintenance)
	versionDataEvacuating = versionData("1", "2", true, "2", api.InMaintenanceEvacuating)
	versionDataEvacuated  = versionData("1", "2", true, "2", api.InMaintenanceEvacuated)
	versionDataRebooting  = versionData("2", "2", true, "2", api.InMaintenanceEvacuated)
	versionDataRebooted   = versionData("2", "2", false, "2", api.InMaintenanceEvacuated)
	versionDataRestoring  = versionData("2", "2", false, "2", api.InMaintenanceRestoring)
	versionDataRestored   = versionData("2", "2", false, "2", api.NotInMaintenance)
)

var (
	versionDataRebootOnlyEvacuating = versionData("1", "1", false, "1", api.InMaintenanceEvacuating)
	versionDataRebootOnlyEvacuated  = versionData("1", "1", false, "1", api.InMaintenanceEvacuated)
	versionDataRebootOnlyRestoring  = versionData("1", "1", false, "1", api.InMaintenanceRestoring)
	versionDataRebootOnlyRestored   = versionData("1", "1", false, "1", api.NotInMaintenance)
)

// rollingUpdateServerClient returns a client mock, which drives the given world
// through a rolling cluster update with reboot.
func rollingUpdateServerClient(world *serverWorld) *adapterMock.ServerClientPortMock {
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
}

// serverWorld is the state of the fake servers, the ServerClientPortMock serves.
//
// State transitions, which a real server completes asynchronously, are released
// explicitly by the test.
type serverWorld struct {
	mu          sync.Mutex
	versionData map[string]api.ServerVersionData
	rebooting   map[string]bool
	pending     []serverWorldTransition
}

// serverWorldTransition is the state a server reaches once it has completed an
// action, that has been triggered on it.
type serverWorldTransition struct {
	server      string
	versionData api.ServerVersionData
	rebooting   bool

	// callback, if set, is invoked after the transition has been applied. It is
	// invoked without holding the lock, since it queries the server again.
	callback func(ctx context.Context, err error)
}

func newServerWorld(versionData map[string]api.ServerVersionData) *serverWorld {
	return &serverWorld{
		versionData: versionData,
		rebooting:   map[string]bool{},
	}
}

func (w *serverWorld) getVersionData(name string) api.ServerVersionData {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Return a copy, the caller enriches the version data with calculated fields.
	versionData := w.versionData[name]
	versionData.Applications = slices.Clone(versionData.Applications)

	return versionData
}

func (w *serverWorld) isRebooting(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.rebooting[name]
}

// set applies the state, a server reaches immediately when an action is triggered
// on it.
func (w *serverWorld) set(name string, versionData api.ServerVersionData, rebooting bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.versionData[name] = versionData
	w.rebooting[name] = rebooting
}

// deferTransition records the state, the server reaches once it has completed the action.
func (w *serverWorld) deferTransition(transition serverWorldTransition) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, transition)
}

// pendingCount returns the number of actions, that have been triggered but not yet
// completed.
func (w *serverWorld) pendingCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return len(w.pending)
}

// release completes the action, that has been triggered first.
func (w *serverWorld) release(ctx context.Context) {
	w.mu.Lock()

	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}

	transition := w.pending[0]
	w.pending = w.pending[1:]

	w.versionData[transition.server] = transition.versionData
	w.rebooting[transition.server] = transition.rebooting

	w.mu.Unlock()

	if transition.callback != nil {
		transition.callback(ctx, nil)
	}
}

var clusterUpdateStateRegexp = regexp.MustCompile(`\[\s*(\d+)/\s*(\d+)\]`)

// requireProgressOnlyMovesForward asserts, that the progress reported to the user
// never moves backwards and that the total number of steps stays constant.
func requireProgressOnlyMovesForward(t *testing.T, observed []string) {
	t.Helper()

	require.NotEmpty(t, observed)

	previousStep := 0
	previousTotalSteps := 0

	for i, description := range observed {
		if description == "" {
			continue
		}

		match := clusterUpdateStateRegexp.FindStringSubmatch(description)
		require.Len(t, match, 3, "observation %d (%q) does not report a step", i, description)

		step, err := strconv.Atoi(match[1])
		require.NoError(t, err)

		totalSteps, err := strconv.Atoi(match[2])
		require.NoError(t, err)

		require.GreaterOrEqual(t, step, previousStep, "progress moved backwards at observation %d, observed: %v", i, observed)

		if previousTotalSteps != 0 {
			require.Equal(t, previousTotalSteps, totalSteps, "total number of steps changed at observation %d, observed: %v", i, observed)
		}

		previousStep = step
		previousTotalSteps = totalSteps
	}

	require.NotZero(t, previousStep, "no progress observed at all, observed: %v", observed)
}

// dedupe removes consecutive duplicates and empty entries, so that a sequence of
// observations can be compared against the sequence of states, that is expected to
// be passed through.
func dedupe(in []string) []string {
	out := make([]string, 0, len(in))

	for _, value := range in {
		if value == "" {
			continue
		}

		if len(out) > 0 && out[len(out)-1] == value {
			continue
		}

		out = append(out, value)
	}

	return out
}

var clusterUpdateStateLogRegexp = regexp.MustCompile(`cluster_update_state=(?:"((?:[^"\\]|\\.)*)"|(\S+))`)

// clusterUpdateStatesFromLog extracts the sequence of cluster update states, that
// the control loop reported while acting on the cluster, with consecutive
// duplicates removed.
func clusterUpdateStatesFromLog(t *testing.T, logOutput string) []string {
	t.Helper()

	matches := clusterUpdateStateLogRegexp.FindAllStringSubmatch(logOutput, -1)

	states := make([]string, 0, len(matches))
	for _, match := range matches {
		value := match[1]
		if value == "" {
			value = match[2]
		}

		unquoted, err := strconv.Unquote(`"` + value + `"`)
		require.NoError(t, err)

		states = append(states, unquoted)
	}

	return dedupe(states)
}
