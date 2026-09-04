package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

const (
	// deploymentTick is what the daemon advances between two runs of the control
	// loop. It is at least the window, in which the BMC data is taken as fresh,
	// so a wait always observes the outcome of the trigger before it.
	deploymentTick = config.ServerDeploymentControlLoopInterval

	// deploymentIdleTick is the jump taken, when the deployment did not move at
	// all, so the minute scale deadlines of the wait states are reached without
	// spending a tick per simulated second.
	deploymentIdleTick = time.Minute

	deploymentDriveIterations = 400
)

// The delays the fake hardware takes to complete what has been triggered on it.
// Each of them has to stay well within the timeout of the state observing it,
// while the two, that have to outlast a deadline of the deployment, exceed it.
const (
	worldBootDuration        = 30 * time.Second
	worldBIOSApplyDelay      = 30 * time.Second
	worldFirmwareRebootDelay = config.ServerDeploymentSettleDelay + time.Minute
	worldInstallDuration     = config.ServerDeploymentMinInstallDuration + 2*time.Minute
	worldEarlyRebootDelay    = 4 * time.Minute
	worldMediaReadDuration   = 5 * time.Minute
	worldRegistrationDelay   = 2 * time.Minute
	worldMediaSize           = 4 * config.ServerDeploymentMediaMinBytesRead
)

const (
	worldBootProgressEarly = "PrimaryProcessorInitializationStarted"
	worldBootProgressLate  = "OSRunning"
	worldBMCHost           = "192.168.1.100"
	worldServerName        = "one"
)

// testClock is the clock of the deployment, advanced by the driver instead of
// passing on its own, since every deadline of the deployment is a compile time
// constant on the minute scale.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(start time.Time) *testClock {
	return &testClock{now: start}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// worldEvent is an effect, that the fake hardware completes some time after it
// has been triggered. The driver releases it, once the clock has reached it.
type worldEvent struct {
	due   time.Time
	name  string
	apply func(ctx context.Context, w *bmcWorld) error
}

// bmcWorld is the state of the fake server and its BMC, as the BMC client mock
// serves it. Everything a real BMC reports about the server is derived from it,
// so the deployment observes a machine, that reacts to what it is told.
type bmcWorld struct {
	mu    sync.Mutex
	clock *testClock

	powerOn       bool
	lastResetTime time.Time
	bootProgress  api.BMCBootProgress
	virtualMedia  map[string]api.BMCVirtualMedia
	bootDevice    string

	biosAttributes  map[string]any
	stagedBIOS      map[string]any
	biosTaskPending bool
	biosTaskState   api.BMCTaskState
	biosTaskSeq     int

	secureBootPending bool

	mediaProgress   map[string]provisioning.SeedImageProgress
	mediaReadStart  time.Time
	mediaProgressID provisioning.SeedImageID
	mediaResets     int

	register func(ctx context.Context) error

	events      []worldEvent
	calls       map[string]int
	detachedIDs []string

	// The knobs, that let a row model a BMC or a server behaving differently.
	dropsMediaOnBoot    bool
	installDuration     time.Duration
	ignorePowerOffs     int
	biosApplyDrops      int
	secureBootEnrolls   bool
	noBootProgress      bool
	noLastResetTime     bool
	uploadTransfer      bool
	installViaMediaRead bool
	mediaFromOtherHost  bool
	registers           bool
	registrationDelay   time.Duration
	getDataFails        bool
	forgetsBIOSTask     bool
	rebootsEarly        bool
	haltsAfterInstall   bool
	awaitingPowerOn     bool
	powerOffErrs        queue.Errs
	attachMediaErrs     queue.Errs
}

func newBMCWorld(t *testing.T, clock *testClock, opts ...func(*bmcWorld)) *bmcWorld {
	t.Helper()

	world := &bmcWorld{
		clock:   clock,
		powerOn: true,
		// The server has been up for a while, so the very first power off is
		// what takes it down, not a machine, that happened to be off already.
		lastResetTime:  clock.Now().Add(-time.Hour),
		bootProgress:   api.BMCBootProgress{LastState: worldBootProgressLate, LastStateTime: clock.Now().Add(-time.Hour)},
		biosAttributes: map[string]any{"BootMode": "Legacy", "SecureBoot": "Disabled"},
		virtualMedia: map[string]api.BMCVirtualMedia{
			"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
			"system:1": {
				ID:         "system:1",
				Inserted:   true,
				Image:      "https://oc.example.com:8443/left-behind.iso",
				ImageName:  "left-behind.iso",
				MediaTypes: []string{string(schemas.CDVirtualMediaType), string(schemas.DVDVirtualMediaType)},
			},
		},
		mediaProgress:     map[string]provisioning.SeedImageProgress{},
		calls:             map[string]int{},
		secureBootEnrolls: true,
		registers:         true,
		registrationDelay: worldRegistrationDelay,
	}

	for _, opt := range opts {
		opt(world)
	}

	return world
}

func (w *bmcWorld) callCount(method string) int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.calls[method]
}

func (w *bmcWorld) detachedSinceInstall() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return slices.Clone(w.detachedIDs)
}

func (w *bmcWorld) mediaInserted() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	var inserted []string

	for _, id := range slices.Sorted(maps.Keys(w.virtualMedia)) {
		if w.virtualMedia[id].Inserted {
			inserted = append(inserted, id)
		}
	}

	return inserted
}

func (w *bmcWorld) setGetDataFails(fails bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.getDataFails = fails
}

func (w *bmcWorld) isPoweredOn() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.powerOn
}

// schedule records an effect the fake hardware completes only after some time.
// It has to be called with the lock held.
func (w *bmcWorld) schedule(after time.Duration, name string, apply func(ctx context.Context, w *bmcWorld) error) {
	w.events = append(w.events, worldEvent{
		due:   w.clock.Now().Add(after),
		name:  name,
		apply: apply,
	})
}

// settle completes every effect, whose time has come, in the order the effects
// were triggered.
func (w *bmcWorld) settle(ctx context.Context) error {
	for {
		w.mu.Lock()

		now := w.clock.Now()

		index := slices.IndexFunc(w.events, func(event worldEvent) bool {
			return !event.due.After(now)
		})

		if index < 0 {
			w.mu.Unlock()
			return nil
		}

		event := w.events[index]
		w.events = slices.Delete(w.events, index, index+1)

		w.mu.Unlock()

		err := event.apply(ctx, w)
		if err != nil {
			return err
		}
	}
}

// bootNow starts a boot cycle: the reset time advances and the boot progress
// falls back to the very first state, which is what tells the deployment, that
// the server has rebooted.
func (w *bmcWorld) bootNow() {
	now := w.clock.Now()

	w.lastResetTime = now
	w.bootProgress = api.BMCBootProgress{LastState: worldBootProgressEarly, LastStateTime: now}

	w.schedule(worldBootDuration, "boot progress reaches the operating system", func(ctx context.Context, w *bmcWorld) error {
		w.mu.Lock()
		defer w.mu.Unlock()

		w.bootProgress = api.BMCBootProgress{LastState: worldBootProgressLate, LastStateTime: w.clock.Now()}

		return nil
	})
}

// startInstall models booting the installation media: the installer reads the
// media and, unless the deployment relies on the read progress alone, reboots
// the server when it is done.
func (w *bmcWorld) startInstall() {
	w.mediaReadStart = w.clock.Now()
	w.detachedIDs = nil

	if w.dropsMediaOnBoot {
		for id, media := range w.virtualMedia {
			media.Inserted = false
			media.Image = ""
			media.ImageName = ""
			w.virtualMedia[id] = media
		}
	}

	if w.installViaMediaRead {
		w.schedule(worldMediaReadDuration, "installation media has been read", func(ctx context.Context, w *bmcWorld) error {
			w.mu.Lock()
			defer w.mu.Unlock()

			w.recordMediaProgress()

			return nil
		})

		return
	}

	// A firmware, that still has something to pick up, reboots the server within
	// the first POST cycles, long before the installation could be done.
	if w.rebootsEarly {
		w.schedule(worldEarlyRebootDelay, "firmware rebooted before the installer started", func(ctx context.Context, w *bmcWorld) error {
			w.mu.Lock()
			defer w.mu.Unlock()

			w.bootNow()

			return nil
		})
	}

	installDuration := w.installDuration
	if installDuration == 0 {
		installDuration = worldInstallDuration
	}

	w.schedule(installDuration, "first stage of the installation completed", func(ctx context.Context, w *bmcWorld) error {
		w.mu.Lock()
		defer w.mu.Unlock()

		w.recordMediaProgress()
		w.bootNow()
		w.scheduleRegistration()

		return nil
	})
}

// scheduleRegistration records, that the server comes up on the installed
// system and registers itself with Operations Center. It has to be called with
// the lock held.
func (w *bmcWorld) scheduleRegistration() {
	if !w.registers || w.register == nil {
		return
	}

	register := w.register

	w.schedule(w.registrationDelay, "server registered itself", func(ctx context.Context, w *bmcWorld) error {
		return register(ctx)
	})
}

// recordMediaProgress has to be called with the lock held.
func (w *bmcWorld) recordMediaProgress() {
	if w.mediaProgressID.FingerprintID == "" {
		return
	}

	source := provisioning.SeedImageSource(worldBMCHost)
	if w.mediaFromOtherHost {
		source = provisioning.SeedImageSource("192.168.1.101")
	}

	w.mediaProgress[source] = provisioning.SeedImageProgress{
		ImageID:      w.mediaProgressID,
		Source:       source,
		Size:         worldMediaSize,
		BytesServed:  worldMediaSize,
		BytesCovered: worldMediaSize,
		FirstRead:    w.mediaReadStart,
		LastRead:     w.clock.Now(),
		RequestCount: 42,
	}
}

func (w *bmcWorld) bmcData() api.BMCData {
	powerState := string(schemas.OffPowerState)
	if w.powerOn {
		powerState = string(schemas.OnPowerState)
	}

	virtualMedia := make(map[string]api.BMCVirtualMedia, len(w.virtualMedia))
	for id, media := range w.virtualMedia {
		media.MediaTypes = slices.Clone(media.MediaTypes)
		virtualMedia[id] = media
	}

	data := api.BMCData{
		BMCProtocol:      "Redfish",
		ServerUUID:       "e9de436e-b94e-4aef-8563-883aec84096e",
		ServerPowerState: powerState,
		VirtualMedia:     virtualMedia,
	}

	if !w.noLastResetTime {
		data.ServerLastResetTime = w.lastResetTime
	}

	if !w.noBootProgress {
		data.ServerBootProgress = w.bootProgress
	}

	return data
}

// deploymentBMCClient serves the world through the BMC client port. WaitForTask
// is deliberately left unset, since the deployment never awaits a task monitor
// synchronously, so a regression panics instead of passing unnoticed.
func deploymentBMCClient(t *testing.T, world *bmcWorld) *adapterMock.BMCServerClientPortMock {
	t.Helper()

	monitor := func(w *bmcWorld) *provisioning.BMCTaskMonitor {
		w.biosTaskSeq++

		return &provisioning.BMCTaskMonitor{URI: "https://bmc.local:8443/redfish/v1/TaskService/Tasks/" + string(rune('0'+w.biosTaskSeq%10))}
	}

	return &adapterMock.BMCServerClientPortMock{
		GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["GetData"]++

			if world.getDataFails {
				return api.BMCData{}, boom.Error
			}

			return world.bmcData(), nil
		},

		ServerPowerOffFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["ServerPowerOff"]++

			err := world.powerOffErrs.PopOrNil(t)
			if err != nil {
				return nil, err
			}

			// A BMC accepting the request without the server ever going down is
			// what a wait state has to survive.
			if world.ignorePowerOffs > 0 {
				world.ignorePowerOffs--

				return monitor(world), nil
			}

			world.powerOn = false
			world.bootProgress = api.BMCBootProgress{}

			return monitor(world), nil
		},

		ServerPowerOnFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["ServerPowerOn"]++

			world.powerOn = true
			world.bootNow()

			if world.biosTaskPending {
				world.schedule(worldBIOSApplyDelay, "staged BIOS attributes applied", func(ctx context.Context, w *bmcWorld) error {
					w.mu.Lock()
					defer w.mu.Unlock()

					maps.Copy(w.biosAttributes, w.stagedBIOS)
					w.stagedBIOS = nil
					w.biosTaskPending = false
					w.biosTaskState = api.BMCTaskStateCompleted

					return nil
				})
			}

			if world.secureBootPending {
				world.secureBootPending = false

				world.schedule(worldFirmwareRebootDelay, "firmware rebooted to pick the certificates up", func(ctx context.Context, w *bmcWorld) error {
					w.mu.Lock()
					defer w.mu.Unlock()

					w.bootNow()

					return nil
				})
			}

			if world.awaitingPowerOn {
				world.awaitingPowerOn = false
				world.scheduleRegistration()
			}

			media, ok := world.virtualMedia[world.bootDevice]
			if ok && media.Inserted {
				world.startInstall()
			}

			return monitor(world), nil
		},

		ApplyBIOSAttributesFunc: func(ctx context.Context, server provisioning.Server, attributes map[string]any) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["ApplyBIOSAttributes"]++

			world.biosTaskPending = true
			world.biosTaskState = api.BMCTaskStateRunning

			// A firmware accepting the attributes and silently dropping them is
			// what the verification of a BIOS pass exists for.
			if world.biosApplyDrops > 0 {
				world.biosApplyDrops--
			} else {
				world.stagedBIOS = maps.Clone(attributes)
			}

			return monitor(world), nil
		},

		TaskStateFunc: func(ctx context.Context, server provisioning.Server, taskMonitor *provisioning.BMCTaskMonitor) (api.BMCTaskState, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["TaskState"]++

			// A BMC forgets a task monitor once it has been consumed or has been
			// reset itself, which the deployment has to survive.
			if world.forgetsBIOSTask {
				return api.BMCTaskStateUnknown, nil
			}

			return world.biosTaskState, nil
		},

		BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["BIOSAttributes"]++

			biosAttributes := make([]api.BIOSAttribute, 0, len(world.biosAttributes))
			for _, name := range slices.Sorted(maps.Keys(world.biosAttributes)) {
				biosAttributes = append(biosAttributes, api.BIOSAttribute{
					Name:         name,
					CurrentValue: world.biosAttributes[name],
					Type:         "String",
				})
			}

			return biosAttributes, nil
		},

		ApplySecureBootCertificatesFunc: func(ctx context.Context, server provisioning.Server, secureBoot api.BIOSSecureBoot) (bool, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["ApplySecureBootCertificates"]++
			world.secureBootPending = world.secureBootEnrolls

			return world.secureBootEnrolls, nil
		},

		AttachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["AttachMedia"]++

			err := world.attachMediaErrs.PopOrNil(t)
			if err != nil {
				return nil, err
			}

			transferMethod := "Stream"
			if world.uploadTransfer {
				transferMethod = string(schemas.UploadTransferMethod)
			}

			media := world.virtualMedia[virtualMediaID]
			media.ID = virtualMediaID
			media.Inserted = true
			media.Image = mediaURL
			media.ImageName = path.Base(mediaURL)
			media.ConnectedVia = "URI"
			media.TransferMethod = transferMethod
			world.virtualMedia[virtualMediaID] = media

			if setBootDevice {
				world.bootDevice = virtualMediaID
			}

			return monitor(world), nil
		},

		DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["DetachMedia"]++
			world.detachedIDs = append(world.detachedIDs, virtualMediaID)

			media := world.virtualMedia[virtualMediaID]
			media.Inserted = false
			media.Image = ""
			media.ImageName = ""
			world.virtualMedia[virtualMediaID] = media

			if world.bootDevice == virtualMediaID {
				world.bootDevice = ""
			}

			// A server, that does not reboot on its own when the first stage of
			// the installation is done, comes up on the installed system once the
			// media it booted from is gone.
			if world.installViaMediaRead && !world.mediaReadStart.IsZero() {
				world.mediaReadStart = time.Time{}

				world.schedule(world.registrationDelay, "server left the first stage of the installation", func(ctx context.Context, w *bmcWorld) error {
					w.mu.Lock()
					defer w.mu.Unlock()

					// A server, that shuts down instead of rebooting, has to be
					// powered on again by the reboot wait.
					if w.haltsAfterInstall {
						w.powerOn = false
						w.bootProgress = api.BMCBootProgress{}
						w.awaitingPowerOn = true

						return nil
					}

					w.bootNow()
					w.scheduleRegistration()

					return nil
				})
			}

			return monitor(world), nil
		},
	}
}

func deploymentSeedImageProgressPort(world *bmcWorld) *adapterMock.SeedImageProgressPortMock {
	return &adapterMock.SeedImageProgressPortMock{
		GetFunc: func(ctx context.Context, imageID provisioning.SeedImageID, source string) (provisioning.SeedImageProgress, bool) {
			world.mu.Lock()
			defer world.mu.Unlock()

			progress, ok := world.mediaProgress[source]
			if !ok || progress.ImageID != imageID {
				return provisioning.SeedImageProgress{}, false
			}

			return progress, true
		},
		GetByImageFunc: func(ctx context.Context, imageID provisioning.SeedImageID) []provisioning.SeedImageProgress {
			world.mu.Lock()
			defer world.mu.Unlock()

			var recorded []provisioning.SeedImageProgress

			for _, source := range slices.Sorted(maps.Keys(world.mediaProgress)) {
				if world.mediaProgress[source].ImageID == imageID {
					recorded = append(recorded, world.mediaProgress[source])
				}
			}

			return recorded
		},
		ResetFunc: func(ctx context.Context, imageID provisioning.SeedImageID) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.mediaResets++
			world.mediaProgressID = imageID
			world.mediaProgress = map[string]provisioning.SeedImageProgress{}
		},
	}
}

// deploymentTestWorld ties the fake hardware to a server service backed by a
// real SQLite schema, so everything the deployment persists really goes through
// the repository it goes through in production.
type deploymentTestWorld struct {
	clock      *testClock
	world      *bmcWorld
	repo       provisioning.ServerRepo
	service    provisioning.ServerService
	newService func() provisioning.ServerService
	logBuf     *bytes.Buffer
	tokenUUID  uuid.UUID
}

type deploymentWorldConfig struct {
	forceReboot  bool
	resolution   *provisioning.BIOSProfileResolution
	trackMedia   bool
	worldOptions []func(*bmcWorld)
}

func setupDeploymentWorld(t *testing.T, ctx context.Context, cfg deploymentWorldConfig) *deploymentTestWorld {
	t.Helper()

	config.InitTest(t, &envMock.EnvironmentMock{
		IsIncusOSFunc: func() bool { return false },
	}, nil)

	err := config.UpdateNetwork(ctx, system.NetworkPut{
		OperationsCenterAddress: deploymentTestOperationsCenterAddress,
		RestServerAddress:       "[::]:8443",
	})
	require.NoError(t, err)

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

	serverDB := sqlite.NewServer(tx)

	server := deploymentTestServer(worldServerName)
	server.BMCConfig.Endpoint = "https://" + worldBMCHost + ":8443"

	_, err = serverDB.Create(ctx, server)
	require.NoError(t, err)

	clock := newTestClock(deploymentTestDate)
	world := newBMCWorld(t, clock, cfg.worldOptions...)

	world.register = func(ctx context.Context) error {
		stored, err := serverDB.GetByName(ctx, worldServerName)
		if err != nil {
			return err
		}

		stored.Status = api.ServerStatusPending
		stored.StatusDetail = api.ServerStatusDetailPendingRegistering
		stored.Type = api.ServerTypeIncus
		stored.ConnectionURL = "https://" + worldServerName + "/"
		stored.Certificate = new(testcert.ClientCertificate)
		stored.LastSeen = clock.Now()
		stored.LastStatusUpdated = clock.Now()

		return serverDB.Update(ctx, *stored)
	}

	tokenUUID := uuidgen.FromPattern(t, "1")

	tokenSvc := &svcMock.TokenServiceMock{
		GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
			return &provisioning.Token{
				UUID:          id,
				UsesRemaining: 1,
				ExpireAt:      clock.Now().Add(24 * time.Hour),
			}, nil
		},
		GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
			return &provisioning.TokenSeed{
				Token:  id,
				Name:   name,
				Public: true,
				Seeds: provisioning.TokenImageSeedConfigs{
					Install: api.SeedInstall{ForceReboot: cfg.forceReboot},
				},
			}, nil
		},
		ResolveTokenSeedImageIDFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
			return "fingerprint-1", nil
		},
	}

	channelSvc := &svcMock.ChannelServiceMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
			return &provisioning.Channel{}, nil
		},
	}

	biosProfile := &adapterMock.BIOSProfilePortMock{
		ResolveFunc: func(ctx context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
			return cfg.resolution, nil
		},
	}

	bmcClient := deploymentBMCClient(t, world)

	newService := func() provisioning.ServerService {
		opts := []provisioningServer.Option{
			provisioningServer.WithNow(clock.Now),
			provisioningServer.WithBIOSProfilePort(biosProfile),
			provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
		}

		if cfg.trackMedia {
			opts = append(opts, provisioningServer.WithSeedImageProgressPort(deploymentSeedImageProgressPort(world)))
		}

		return provisioningServer.New(serverDB, nil, nil, tokenSvc, nil, channelSvc, nil, tls.Certificate{}, opts...)
	}

	return &deploymentTestWorld{
		clock:      clock,
		world:      world,
		repo:       serverDB,
		service:    newService(),
		newService: newService,
		logBuf:     logBuf,
		tokenUUID:  tokenUUID,
	}
}

// deploymentStateSequence returns the states the deployment has passed through,
// in order, ending in the state it is in. A retry does not transition and a
// fallback always targets a trigger state, so the sequence never repeats a state
// back to back and can be compared as is.
func deploymentStateSequence(server provisioning.Server) []api.ServerDeploymentState {
	deployment := server.StatusInternal.Deployment

	states := make([]api.ServerDeploymentState, 0, len(deployment.History)+1)
	for _, step := range deployment.History {
		states = append(states, step.State)
	}

	return append(states, deployment.State)
}

type deploymentDriveHook func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server)

// driveDeployment runs the control loop until the deployment reaches a terminal
// state, advancing the clock by a tick, when the deployment moved, and by a
// larger step, when it did not, so the minute scale deadlines are reached
// without simulating every second in between.
func driveDeployment(t *testing.T, ctx context.Context, w *deploymentTestWorld, rebuildService bool, hooks ...deploymentDriveHook) provisioning.Server {
	t.Helper()

	for range deploymentDriveIterations {
		server, err := w.repo.GetByName(ctx, worldServerName)
		require.NoError(t, err)

		if !server.StatusInternal.Deployment.IsActive() {
			return *server
		}

		before := server.StatusInternal.Deployment.State

		svc := w.service
		if rebuildService {
			svc = w.newService()
		}

		for _, hook := range hooks {
			hook(t, ctx, svc, *server)
		}

		require.NoError(t, w.world.settle(ctx), "the fake hardware failed to settle in state %q", before)

		err = svc.DeploymentControlLoop(ctx, nil)
		require.NoError(t, err, "the control loop failed in state %q", before)

		after, err := w.repo.GetByName(ctx, worldServerName)
		require.NoError(t, err)

		advance := deploymentTick
		if after.StatusInternal.Deployment.State == before {
			advance = deploymentIdleTick
		}

		w.clock.advance(advance)
	}

	server, err := w.repo.GetByName(ctx, worldServerName)
	require.NoError(t, err)

	require.FailNow(
		t, "the deployment did not reach a terminal state",
		"stopped in %q after %d iterations, passed through %v",
		server.StatusInternal.Deployment.State, deploymentDriveIterations, deploymentStateSequence(*server),
	)

	return provisioning.Server{}
}

// The state groups the deployment passes through, so an expected sequence is the
// happy path with the groups a row passes by left out.
var (
	deploymentStatesPreparing = []api.ServerDeploymentState{
		api.ServerDeploymentStateRefreshBMCData,
		api.ServerDeploymentStateCheckBIOS,
	}

	deploymentStatesBIOSPass = []api.ServerDeploymentState{
		api.ServerDeploymentStatePowerOffBIOS,
		api.ServerDeploymentStateWaitPowerOffBIOS,
		api.ServerDeploymentStateApplyBIOS,
		api.ServerDeploymentStatePowerOnBIOS,
		api.ServerDeploymentStateWaitBIOSApplied,
		api.ServerDeploymentStateVerifyBIOS,
	}

	deploymentStatesBIOSDeferredPass = []api.ServerDeploymentState{
		api.ServerDeploymentStatePowerOffBIOSDeferred,
		api.ServerDeploymentStateWaitPowerOffBIOSDeferred,
		api.ServerDeploymentStateApplyBIOSDeferred,
		api.ServerDeploymentStatePowerOnBIOSDeferred,
		api.ServerDeploymentStateWaitBIOSAppliedDeferred,
		api.ServerDeploymentStateVerifyBIOSDeferred,
	}

	deploymentStatesSecureBootOff = []api.ServerDeploymentState{
		api.ServerDeploymentStatePowerOffSecureBoot,
		api.ServerDeploymentStateWaitPowerOffSecureBoot,
	}

	deploymentStatesSecureBoot = []api.ServerDeploymentState{
		api.ServerDeploymentStateSecureBoot,
	}

	deploymentStatesMediaCleared = []api.ServerDeploymentState{
		api.ServerDeploymentStateClearMedia,
		api.ServerDeploymentStateWaitMediaCleared,
	}

	deploymentStatesSecureBootSettle = []api.ServerDeploymentState{
		api.ServerDeploymentStatePowerOnSecureBoot,
		api.ServerDeploymentStateWaitSecureBootSettled,
		api.ServerDeploymentStatePowerOffSecureBootSettled,
		api.ServerDeploymentStateWaitPowerOffSecureBootSettled,
	}

	deploymentStatesInstall = []api.ServerDeploymentState{
		api.ServerDeploymentStateAttachMedia,
		api.ServerDeploymentStateWaitMediaAttached,
		api.ServerDeploymentStatePowerOnInstall,
		api.ServerDeploymentStateWaitInstall,
	}

	deploymentStatesFinalize = []api.ServerDeploymentState{
		api.ServerDeploymentStateDetachMedia,
		api.ServerDeploymentStateWaitMediaDetached,
		api.ServerDeploymentStateWaitReboot,
		api.ServerDeploymentStateWaitRegistration,
		api.ServerDeploymentStateCleanup,
		api.ServerDeploymentStateCompleted,
	}

	deploymentStatesCancel = []api.ServerDeploymentState{
		api.ServerDeploymentStateCancel,
		api.ServerDeploymentStateWaitCancel,
		api.ServerDeploymentStateCancelled,
	}
)

func deploymentStatesHappyPath() []api.ServerDeploymentState {
	return slices.Concat(
		deploymentStatesPreparing,
		deploymentStatesBIOSPass,
		deploymentStatesBIOSDeferredPass,
		deploymentStatesSecureBootOff,
		deploymentStatesSecureBoot,
		deploymentStatesMediaCleared,
		deploymentStatesSecureBootSettle,
		deploymentStatesInstall,
		deploymentStatesFinalize,
	)
}
