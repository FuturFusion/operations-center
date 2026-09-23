package server_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/securebootcerts"
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

	// deploymentMaxMediaEjectDelay is how long a server, that has installed and
	// waits for its media to go before it reboots, may be left waiting. It bounds
	// the media idle period plus the granularity, at which the fake advances.
	deploymentMaxMediaEjectDelay = 3 * time.Minute

	deploymentDriveIterations = 400
)

// The delays the fake hardware takes to complete what has been triggered on it.
// Each of them has to stay well within the timeout of the state observing it,
// while the two, that have to outlast a deadline of the deployment, exceed it.
const (
	worldBootDuration           = 30 * time.Second
	worldBIOSApplyDelay         = 30 * time.Second
	worldFirmwareRebootDelay    = config.ServerDeploymentSettleDelay + time.Minute
	worldInstallDuration        = config.ServerDeploymentMinInstallDuration + 2*time.Minute
	worldEarlyRebootDelay       = 4 * time.Minute
	worldMediaReadDuration      = 5 * time.Minute
	worldRegistrationDelay      = 2 * time.Minute
	worldEjectDelay             = 30 * time.Second
	worldMediaSize              = 4 * config.ServerDeploymentMediaMinBytesRead
	worldIgnorePowerOffDuration = config.ServerDeploymentStepTimeout + time.Minute
)

const (
	worldBootProgressEarly = "PrimaryProcessorInitializationStarted"
	worldBootProgressLate  = "OSRunning"
	worldBMCHost           = "192.168.1.100"
	worldServerName        = "one"

	worldSecureBootModeSetup = string(schemas.SetupModeSecureBootModeType)
	worldSecureBootModeUser  = string(schemas.UserModeSecureBootModeType)

	worldSecureBootMediaPathSegment = "/secure-boot-media/"
	worldSecureBootMediaID          = "AAAAAAAAAAAA"
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

	secureBootMode         string
	secureBootResetPending bool

	secureBootResetTaskURI   string
	secureBootResetTaskState api.BMCTaskState
	poweredOnWhileResetting  bool

	secureBootMediaImageType    api.ImageType
	secureBootMediaArchitecture images.UpdateFileArchitecture

	processorArchitecture   string
	processorInstructionSet string

	mediaProgress     map[string]provisioning.SeedImageProgress
	mediaReadStart    time.Time
	mediaDeploymentID string
	mediaResets       int

	register func(ctx context.Context) error

	events      []worldEvent
	calls       map[string]int
	detachedIDs []string

	// The knobs, that let a row model a BMC or a server behaving differently.
	dropsMediaOnBoot              bool
	installDuration               time.Duration
	postDuration                  time.Duration
	bootGeneration                int
	bootsMediaAgain               bool
	ignorePowerOffFor             time.Duration
	ignoredPowerOffsSince         time.Time
	biosApplyDrops                int
	secureBootEnrolls             bool
	relapsesAfterSecureBootReset  bool
	biosSecureBootStatusAttribute string
	biosSecureBootStatusStuck     bool
	noSecureBootMode              bool
	noSecureBootReset             bool
	noBootProgress                bool
	noLastResetTime               bool
	uploadTransfer                bool
	installViaMediaRead           bool
	cachesMedia                   bool
	mediaEjectDelay               time.Duration
	mediaFromOtherHost            bool
	registers                     bool
	registrationDelay             time.Duration
	getDataFails                  bool
	unavailableParts              map[api.BMCDataPart]string
	forgetsBIOSTask               bool
	slowSecureBootReset           bool
	rebootsEarly                  bool
	haltsAfterInstall             bool
	awaitingPowerOn               bool
	powerOffErrs                  queue.Errs
	attachMediaErrs               queue.Errs
	enableSecureBootErrs          queue.Errs
	ejectDelay                    time.Duration
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
		mediaProgress:           map[string]provisioning.SeedImageProgress{},
		calls:                   map[string]int{},
		secureBootMode:          worldSecureBootModeUser,
		processorArchitecture:   "x86",
		processorInstructionSet: "x86-64",
		secureBootEnrolls:       true,
		registers:               true,
		registrationDelay:       worldRegistrationDelay,
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

func (w *bmcWorld) mediaEjectedAfter() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.mediaEjectDelay
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

// setPartUnavailable makes the BMC answer for everything but the given part, the
// way a BMC does, whose data sources are temporarily unavailable. The fields of
// the part keep whatever the world holds, so a test can tell a caller deciding
// on them apart from one, that establishes the part was collected first.
func (w *bmcWorld) setPartUnavailable(part api.BMCDataPart, reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.unavailableParts == nil {
		w.unavailableParts = map[api.BMCDataPart]string{}
	}

	w.unavailableParts[part] = reason
}

func (w *bmcWorld) setPartAvailable(part api.BMCDataPart) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.unavailableParts, part)
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
	w.bootGeneration++

	postDuration := w.postDurationOrDefault()

	generation := w.bootGeneration

	w.schedule(postDuration, "boot progress reaches the operating system", func(ctx context.Context, w *bmcWorld) error {
		w.mu.Lock()
		defer w.mu.Unlock()

		if w.bootGeneration != generation {
			return nil
		}

		w.bootProgress = api.BMCBootProgress{LastState: worldBootProgressLate, LastStateTime: w.clock.Now()}

		return nil
	})
}

// postDurationOrDefault returns how long the server takes from a reset to the
// hand over to the operating system. It has to be called with the lock held.
func (w *bmcWorld) postDurationOrDefault() time.Duration {
	if w.postDuration == 0 {
		return worldBootDuration
	}

	return w.postDuration
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

	// A BMC, that caches the installation media instead of streaming it to the
	// installer, has read it out while the server boots and stays quiet for the
	// rest of the installation.
	if w.cachesMedia {
		w.schedule(worldMediaReadDuration, "installation media has been read", func(ctx context.Context, w *bmcWorld) error {
			w.mu.Lock()
			defer w.mu.Unlock()

			w.recordMediaProgress()

			return nil
		})
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

		if !w.cachesMedia {
			w.recordMediaProgress()
		}

		w.bootNow()

		// A server, that does not fall back to the installed system on its own,
		// boots the installation media again, where the installer refuses to run
		// a second time, so the server never registers. Ejecting the media has
		// the POST of that boot to come through.
		if w.bootsMediaAgain {
			w.schedule(w.postDurationOrDefault(), "server picked its boot device", func(ctx context.Context, w *bmcWorld) error {
				w.mu.Lock()
				defer w.mu.Unlock()

				media, ok := w.virtualMedia[w.bootDevice]
				if ok && media.Inserted {
					return nil
				}

				w.scheduleRegistration()

				return nil
			})

			return nil
		}

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
	if w.mediaDeploymentID == "" {
		return
	}

	w.mediaProgress[w.mediaDeploymentID] = provisioning.SeedImageProgress{
		DeploymentID: w.mediaDeploymentID,
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

	if !w.noSecureBootMode {
		data.ServerSecureBootMode = w.secureBootMode
	}

	data.ServerProcessorArchitecture = w.processorArchitecture
	data.ServerProcessorInstructionSet = w.processorInstructionSet

	// A part, the BMC did not answer for, is left at its zero value and named,
	// the same way the Redfish adapter records it.
	if len(w.unavailableParts) > 0 {
		data.Unavailable = maps.Clone(w.unavailableParts)

		for part := range w.unavailableParts {
			switch part {
			case api.BMCDataPartVirtualMedia:
				data.VirtualMedia = nil

			case api.BMCDataPartSystem:
				data.ServerPowerState = ""
				data.ServerLastResetTime = time.Time{}
				data.ServerBootProgress = api.BMCBootProgress{}
				data.ServerSecureBootMode = ""
			}
		}
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
			if world.ignorePowerOffFor > 0 {
				if world.ignoredPowerOffsSince.IsZero() {
					world.ignoredPowerOffsSince = world.clock.Now()
				}

				if world.clock.Now().Before(world.ignoredPowerOffsSince.Add(world.ignorePowerOffFor)) {
					return monitor(world), nil
				}
			}

			world.powerOn = false
			world.bootProgress = api.BMCBootProgress{}

			if world.relapsesAfterSecureBootReset && world.secureBootMode == worldSecureBootModeSetup {
				world.relapsesAfterSecureBootReset = false

				world.schedule(worldBootDuration, "firmware brings the server back up to pick the cleared key databases up", func(ctx context.Context, w *bmcWorld) error {
					w.mu.Lock()
					defer w.mu.Unlock()

					w.powerOn = true
					w.bootNow()

					return nil
				})
			}

			return monitor(world), nil
		},

		ServerPowerOnFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["ServerPowerOn"]++

			if world.secureBootResetTaskState == api.BMCTaskStateRunning {
				world.poweredOnWhileResetting = true
			}

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

			if world.secureBootResetPending {
				world.secureBootResetPending = false

				world.schedule(worldFirmwareRebootDelay, "firmware cleared the secure boot key databases", func(ctx context.Context, w *bmcWorld) error {
					w.mu.Lock()
					defer w.mu.Unlock()

					w.secureBootMode = worldSecureBootModeSetup
					w.bootNow()

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
				if strings.Contains(media.Image, worldSecureBootMediaPathSegment) {
					if world.secureBootMode == worldSecureBootModeSetup {
						world.schedule(worldFirmwareRebootDelay, "enrollment media enrolled the secure boot certificates", func(ctx context.Context, w *bmcWorld) error {
							w.mu.Lock()
							defer w.mu.Unlock()

							w.secureBootMode = worldSecureBootModeUser
							w.bootNow()

							return nil
						})
					}
				} else {
					world.startInstall()
				}
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

			if taskMonitor != nil && taskMonitor.URI == world.secureBootResetTaskURI && world.secureBootResetTaskState != "" {
				state := world.secureBootResetTaskState
				world.secureBootResetTaskState = api.BMCTaskStateCompleted

				return state, nil
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

		ResetSecureBootKeysFunc: func(ctx context.Context, server provisioning.Server) (bool, *provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["ResetSecureBootKeys"]++

			if world.noSecureBootReset {
				return false, nil, fmt.Errorf("Resetting the secure boot keys is not supported: %w", domain.ErrOperationNotPermitted)
			}

			if world.secureBootMode == worldSecureBootModeSetup {
				return false, nil, nil
			}

			world.secureBootResetPending = true

			taskMonitor := monitor(world)
			world.secureBootResetTaskURI = taskMonitor.URI

			if world.slowSecureBootReset {
				world.secureBootResetTaskState = api.BMCTaskStateRunning
			}

			return true, taskMonitor, nil
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

			if !strings.Contains(mediaURL, worldSecureBootMediaPathSegment) {
				world.mediaDeploymentID = deploymentIDFromMediaURL(t, mediaURL)
			}

			return monitor(world), nil
		},

		DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["DetachMedia"]++
			world.detachedIDs = append(world.detachedIDs, virtualMediaID)

			lastRead := world.mediaProgress[world.mediaDeploymentID].LastRead
			if !lastRead.IsZero() {
				world.mediaEjectDelay = world.clock.Now().Sub(lastRead)
			}

			eject := func(w *bmcWorld) {
				media := w.virtualMedia[virtualMediaID]
				media.Inserted = false
				media.Image = ""
				media.ImageName = ""
				w.virtualMedia[virtualMediaID] = media

				if w.bootDevice == virtualMediaID {
					w.bootDevice = ""
				}
			}

			if world.ejectDelay > 0 {
				world.schedule(world.ejectDelay, "media of "+virtualMediaID+" ejected", func(ctx context.Context, w *bmcWorld) error {
					w.mu.Lock()
					defer w.mu.Unlock()

					eject(w)

					return nil
				})
			} else {
				eject(world)
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

func deploymentIDFromMediaURL(t *testing.T, mediaURL string) string {
	t.Helper()

	parsed, err := url.Parse(mediaURL)
	require.NoError(t, err)

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	i := slices.Index(segments, "deployment")
	require.NotEqual(t, -1, i, "the media URL names no deployment: %s", mediaURL)
	require.Less(t, i+1, len(segments), "the media URL names no deployment: %s", mediaURL)

	return segments[i+1]
}

func deploymentSeedImageProgressPort(world *bmcWorld) *adapterMock.SeedImageProgressPortMock {
	return &adapterMock.SeedImageProgressPortMock{
		GetFunc: func(ctx context.Context, deploymentID string) (provisioning.SeedImageProgress, bool) {
			world.mu.Lock()
			defer world.mu.Unlock()

			progress, ok := world.mediaProgress[deploymentID]

			return progress, ok
		},
		ResetFunc: func(ctx context.Context, deploymentID string) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.mediaResets++

			delete(world.mediaProgress, deploymentID)
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
	forceReboot bool
	resolution  *provisioning.BIOSProfileResolution
	trackMedia  bool

	noSecureBootMedia bool

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

		if !cfg.noSecureBootMedia {
			opts = append(opts, provisioningServer.WithSecureBootMediaPort(
				deploymentSecureBootMediaPort(world),
				deploymentSecureBootCertificateSource(),
				deploymentSecureBootCatalog(t),
			))
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

func deploymentSecureBootMediaPort(world *bmcWorld) *adapterMock.SecureBootMediaPortMock {
	return &adapterMock.SecureBootMediaPortMock{
		CheckSupportedFunc: func(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture) error {
			return nil
		},
		GenerateFunc: func(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture, certificates provisioning.SecureBootCertificates) (string, error) {
			world.mu.Lock()
			defer world.mu.Unlock()

			world.calls["GenerateSecureBootMedia"]++
			world.secureBootMediaImageType = imageType
			world.secureBootMediaArchitecture = architecture

			return worldSecureBootMediaID, nil
		},
	}
}

func deploymentSecureBootCertificateSource() *adapterMock.SecureBootCertificateSourcePortMock {
	return &adapterMock.SecureBootCertificateSourcePortMock{
		GetSecureBootCertificatesFunc: func(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error) {
			return incusosapi.InternalSecureBootCertificates{
				PK:  "PK certificate",
				KEK: []string{"KEK certificate"},
				DB:  []string{"DB certificate"},
			}, nil
		},
		GetSecureBootPlatformKeyUpdateFunc: func(ctx context.Context) ([]byte, error) {
			return []byte("PK update"), nil
		},
	}
}

func deploymentSecureBootCatalog(t *testing.T) provisioning.SecureBootCertificateCatalogPort {
	t.Helper()

	catalog, err := securebootcerts.New()
	require.NoError(t, err)

	return catalog
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

	deploymentStatesSecureBootReset = []api.ServerDeploymentState{
		api.ServerDeploymentStateResetSecureBootKeys,
		api.ServerDeploymentStateWaitSecureBootReset,
		api.ServerDeploymentStatePowerOnSecureBootReset,
		api.ServerDeploymentStateWaitSecureBootSetupMode,
		api.ServerDeploymentStatePowerOffSecureBootReset,
		api.ServerDeploymentStateWaitPowerOffSecureBootReset,
	}

	deploymentStatesSecureBootMedia = []api.ServerDeploymentState{
		api.ServerDeploymentStateAttachSecureBootMedia,
		api.ServerDeploymentStateWaitSecureBootMediaAttached,
		api.ServerDeploymentStatePowerOnSecureBootMedia,
		api.ServerDeploymentStateWaitSecureBootEnrolled,
		api.ServerDeploymentStatePowerOffSecureBootMedia,
		api.ServerDeploymentStateWaitPowerOffSecureBootMedia,
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

	deploymentStatesCancelSkipCleanup = []api.ServerDeploymentState{
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
