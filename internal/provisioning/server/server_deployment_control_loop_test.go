package server_test

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

var (
	deploymentTestBIOSAttributes         = map[string]any{"BootMode": "Uefi"}
	deploymentTestBIOSDeferredAttributes = map[string]any{"SecureBoot": "Enabled"}
)

func deploymentTestResolution() *provisioning.BIOSProfileResolution {
	return &provisioning.BIOSProfileResolution{
		Profiles:           []string{"generic"},
		Attributes:         maps.Clone(deploymentTestBIOSAttributes),
		DeferredAttributes: maps.Clone(deploymentTestBIOSDeferredAttributes),
	}
}

func deploymentTestRequest(tokenUUID uuid.UUID) provisioning.ServerDeploymentRequest {
	return provisioning.ServerDeploymentRequest{
		TokenUUID:    tokenUUID,
		Seed:         "default",
		ImageType:    api.ImageTypeISO,
		Architecture: images.UpdateFileArchitecture64BitX86,
	}
}

func TestServerService_DeploymentControlLoopDrivesDeploymentToATerminalState(t *testing.T) {
	tests := []struct {
		name           string
		forceReboot    bool
		resolution     *provisioning.BIOSProfileResolution
		trackMedia     bool
		worldOptions   []func(*bmcWorld)
		request        func(request *provisioning.ServerDeploymentRequest)
		rebuildService bool
		cancelAt       api.ServerDeploymentState
		cancelTwice    bool

		wantStates           []api.ServerDeploymentState
		wantStatus           api.ServerStatus
		wantStatusDetail     api.ServerStatusDetail
		wantFailedState      api.ServerDeploymentState
		wantFallbackAttempts int
		wantRetries          int
		wantLastError        string
		assertWorld          func(t *testing.T, world *bmcWorld)
		assertLog            log.MatcherFunc
	}{
		{
			name:        "success - the full deployment",
			forceReboot: true,
			resolution:  deploymentTestResolution(),

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Empty(t, world.mediaInserted(), "the clean up ejects the installation media")
				require.Equal(t, map[string]any{"BootMode": "Uefi", "SecureBoot": "Enabled"}, world.biosAttributes)
			},
		},
		{
			name:        "success - the BIOS attributes are applied already",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.biosAttributes = map[string]any{"BootMode": "Uefi", "SecureBoot": "Enabled"} },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("BIOS attributes are applied already, passing the first BIOS pass by"),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Zero(t, world.callCount("ApplyBIOSAttributes"), "an already configured server is not power cycled for nothing")
			},
		},
		{
			name:        "success - only the deferred BIOS pass is pending",
			forceReboot: true,
			resolution: &provisioning.BIOSProfileResolution{
				Profiles:           []string{"generic"},
				DeferredAttributes: maps.Clone(deploymentTestBIOSDeferredAttributes),
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name:        "success - no BIOS profile attributes at all",
			forceReboot: true,
			resolution:  &provisioning.BIOSProfileResolution{Profiles: []string{"generic"}},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name:        "success - the secure boot enrollment is skipped",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SkipSecureBootCertificates = true
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesMediaCleared,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Zero(t, world.callCount("ApplySecureBootCertificates"))
			},
		},
		{
			name:        "success - the secure boot certificates are enrolled already",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.secureBootEnrolls = false },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, 1, world.callCount("ApplySecureBootCertificates"), "the enrollment is attempted, it just writes nothing")
			},
		},
		{
			name:       "success - the installation is detected from the read progress of the media",
			resolution: deploymentTestResolution(),
			trackMedia: true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.installViaMediaRead = true },
			},
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.Force = true
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Installation completed, the installation media has been read and is idle"),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, 2, world.mediaResets, "the read progress is dropped when the media is attached and again when it is cleaned up")
			},
		},
		{
			name:       "success - the BMC reads the installation media from another address",
			resolution: deploymentTestResolution(),
			trackMedia: true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.installViaMediaRead = true; w.mediaFromOtherHost = true },
			},
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.Force = true
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("The BMC reads the installation media from another address"),
		},
		{
			name:        "success - the BMC uploads the installation media instead of streaming it",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			trackMedia:  true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.uploadTransfer = true },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name:        "success - the BMC reports no boot progress",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.noBootProgress = true },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name:        "success - the BMC reports no last reset time",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.noLastResetTime = true },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name:       "success - the BMC can not tell whether the server rebooted at all",
			resolution: deploymentTestResolution(),
			trackMedia: true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) {
					w.noBootProgress = true
					w.noLastResetTime = true
					w.installViaMediaRead = true
					w.registrationDelay = config.ServerDeploymentRebootObservationWindow + 2*time.Minute
				},
			},
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.Force = true
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Reboot of the server could not be observed, falling back to the power state"),
		},
		{
			name:        "success - the BMC forgets the BIOS task monitor",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.forgetsBIOSTask = true },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("BMC does not know the BIOS task monitor anymore, falling back to the power state"),
		},
		{
			name:        "success - the installer finishes before the installation could be done by the clock",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			trackMedia:  true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.installDuration = config.ServerDeploymentMinInstallDuration / 2 },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Installation completed, the BMC reports a reboot of the server"),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Empty(t, world.mediaInserted(), "the installation media is ejected as soon as the reboot is observed")
			},
		},
		{
			name:        "success - the firmware reboots before the installer even started",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.rebootsEarly = true },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Server rebooted before the installation could have completed, waiting for the installation"),
		},
		{
			name:       "success - the server shuts down instead of rebooting after the installation",
			resolution: deploymentTestResolution(),
			trackMedia: true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.installViaMediaRead = true; w.haltsAfterInstall = true },
			},
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.Force = true
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.True(t, world.isPoweredOn(), "the reboot wait powers a server, that stayed off, on again")
			},
		},
		{
			name:        "success - a wait times out once and falls back to its trigger",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.ignorePowerOffs = 1 },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass[:2],
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Deployment wait timed out, falling back to the trigger"),
		},
		{
			name:        "success - the BIOS verification fails once and repeats the pass",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.biosApplyDrops = 1 },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:           api.ServerStatusPending,
			wantStatusDetail:     api.ServerStatusDetailPendingRegistering,
			wantFallbackAttempts: 1,
			assertLog:            log.Contains("Deployment step failed, going back to an earlier state"),
		},
		{
			name:           "success - the service is rebuilt between every tick",
			forceReboot:    true,
			resolution:     deploymentTestResolution(),
			rebuildService: true,

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
		},
		{
			name:        "failure - the BIOS verification keeps failing",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.biosApplyDrops = 10 },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSPass,
				[]api.ServerDeploymentState{api.ServerDeploymentStateFailed},
			),
			wantStatus:           api.ServerStatusUnregistered,
			wantStatusDetail:     api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:      api.ServerDeploymentStateVerifyBIOS,
			wantFallbackAttempts: config.ServerDeploymentStepRetries,
			wantLastError:        "have not been applied",
		},
		{
			name:        "failure - the BMC rejects the media attachment for good",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.attachMediaErrs = queue.Errs{boom.Error} },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				[]api.ServerDeploymentState{
					api.ServerDeploymentStateAttachMedia,
					api.ServerDeploymentStateFailed,
				},
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:  api.ServerDeploymentStateAttachMedia,
			wantLastError:    boom.Error.Error(),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, 1, world.callCount("AttachMedia"), "a non retryable rejection is not attempted again")
			},
		},
		{
			name:        "failure - the BMC keeps rejecting the power off",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) {
					w.powerOffErrs = queue.Errs{
						domain.NewRetryableErr(boom.Error),
						domain.NewRetryableErr(boom.Error),
						domain.NewRetryableErr(boom.Error),
						domain.NewRetryableErr(boom.Error),
					}
				},
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				[]api.ServerDeploymentState{
					api.ServerDeploymentStatePowerOffBIOS,
					api.ServerDeploymentStateFailed,
				},
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:  api.ServerDeploymentStatePowerOffBIOS,
			wantRetries:      config.ServerDeploymentStepRetries,
			wantLastError:    boom.Error.Error(),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, config.ServerDeploymentStepRetries+1, world.callCount("ServerPowerOff"), "the retry budget is spent in full")
			},
		},
		{
			name:        "failure - the server never registers itself",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.registers = false },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				[]api.ServerDeploymentState{
					api.ServerDeploymentStateDetachMedia,
					api.ServerDeploymentStateWaitMediaDetached,
					api.ServerDeploymentStateWaitReboot,
					api.ServerDeploymentStateWaitRegistration,
					api.ServerDeploymentStateFailed,
				},
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:  api.ServerDeploymentStateWaitRegistration,
			wantLastError:    "did not complete within " + config.ServerDeploymentRegistrationTimeout.String(),
		},
		{
			name:        "cancelled - the BMC dropped the media on its own",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			cancelAt:    api.ServerDeploymentStateWaitInstall,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.dropsMediaOnBoot = true },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesCancel,
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentCancelled,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Contains(
					t, world.detachedSinceInstall(), deploymentTestOpticalMedia.ID,
					"the device the media was attached to is detached even with nothing inserted, since that is what takes the boot device override back",
				)
			},
		},
		{
			name:        "cancelled - from an early wait",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			cancelAt:    api.ServerDeploymentStateWaitPowerOffBIOS,
			cancelTwice: true,

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass[:2],
				deploymentStatesCancel,
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentCancelled,
		},
		{
			name:        "cancelled - from the install wait",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			cancelAt:    api.ServerDeploymentStateWaitInstall,

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesCancel,
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentCancelled,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Empty(t, world.mediaInserted(), "a cancelled deployment ejects the installation media")
				require.False(t, world.isPoweredOn(), "a cancelled deployment leaves the server powered off")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			// Setup
			w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
				forceReboot:  tc.forceReboot,
				resolution:   tc.resolution,
				trackMedia:   tc.trackMedia,
				worldOptions: tc.worldOptions,
			})

			request := deploymentTestRequest(w.tokenUUID)
			if tc.request != nil {
				tc.request(&request)
			}

			err := w.service.DeployByName(ctx, worldServerName, request)
			require.NoError(t, err)

			var hooks []deploymentDriveHook

			if tc.cancelAt != "" {
				hooks = append(hooks, func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server) {
					t.Helper()

					if server.StatusInternal.Deployment.State != tc.cancelAt {
						return
					}

					require.NoError(t, svc.CancelDeploymentByName(ctx, worldServerName))

					if tc.cancelTwice {
						require.NoError(t, svc.CancelDeploymentByName(ctx, worldServerName), "cancelling twice is a no-op")
					}
				})
			}

			// Run test
			server := driveDeployment(t, ctx, w, tc.rebuildService, hooks...)

			// Assert
			deployment := server.StatusInternal.Deployment

			require.Equal(t, tc.wantStates, deploymentStateSequence(server))
			require.Equal(t, tc.wantStatus, server.Status)
			require.Equal(t, tc.wantStatusDetail, server.StatusDetail)
			require.Equal(t, tc.wantFailedState, deployment.FailedState)
			require.Equal(t, tc.wantFallbackAttempts, deployment.FallbackAttempts)
			require.False(t, deployment.FinishedAt.IsZero(), "a terminal state stamps the end of the deployment")

			if tc.wantLastError == "" {
				require.Empty(t, deployment.LastError)
			} else {
				require.Contains(t, deployment.LastError, tc.wantLastError, "the live record keeps the reason the deployment failed")
			}

			if tc.wantRetries > 0 {
				failing := deployment.History[len(deployment.History)-1]
				require.Equal(t, tc.wantFailedState, failing.State)
				require.Equal(t, tc.wantRetries, failing.Retries, "the retry budget the failing state spent")
			}

			if tc.assertLog != nil {
				tc.assertLog(t, w.logBuf)
			}

			if tc.assertWorld != nil {
				tc.assertWorld(t, w.world)
			}
		})
	}
}

// TestServerService_DeploymentControlLoopSurvivesAServiceRestart drives the
// deployment with one service until the longest wait, then hands it over to a
// service, that has never seen it before, so everything it needs has to come out
// of the server record.
func TestServerService_DeploymentControlLoopSurvivesAServiceRestart(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	for range deploymentDriveIterations {
		server, err := w.repo.GetByName(ctx, worldServerName)
		require.NoError(t, err)

		deployment := server.StatusInternal.Deployment
		if deployment.State == api.ServerDeploymentStateWaitInstall && !deployment.InstallSnapshot.Taken.IsZero() {
			break
		}

		require.NoError(t, w.world.settle(ctx))
		require.NoError(t, w.service.DeploymentControlLoop(ctx, nil))

		after, err := w.repo.GetByName(ctx, worldServerName)
		require.NoError(t, err)

		advance := deploymentTick
		if after.StatusInternal.Deployment.State == deployment.State {
			advance = deploymentIdleTick
		}

		w.clock.advance(advance)
	}

	restarted, err := w.repo.GetByName(ctx, worldServerName)
	require.NoError(t, err)
	require.Equal(t, api.ServerDeploymentStateWaitInstall, restarted.StatusInternal.Deployment.State, "the hand over happens in the longest wait")
	require.False(t, restarted.StatusInternal.Deployment.InstallSnapshot.Taken.IsZero(), "the install wait has anchored its reboot detection")

	w.service = w.newService()

	server := driveDeployment(t, ctx, w, false)

	require.Equal(t, deploymentStatesHappyPath(), deploymentStateSequence(server))
	require.Equal(t, api.ServerStatusPending, server.Status)
}

// TestServerService_DeploymentControlLoopLeavesAFailedDeploymentAlone asserts,
// that a failed deployment is not cleaned up, so an operator can look at the
// server the way the deployment left it.
func TestServerService_DeploymentControlLoopLeavesAFailedDeploymentAlone(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
		worldOptions: []func(*bmcWorld){
			func(world *bmcWorld) {
				world.powerOffErrs = queue.Errs{
					domain.NewRetryableErr(boom.Error),
					domain.NewRetryableErr(boom.Error),
					domain.NewRetryableErr(boom.Error),
					domain.NewRetryableErr(boom.Error),
				}
			},
		},
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	server := driveDeployment(t, ctx, w, false)

	require.Equal(t, api.ServerDeploymentStateFailed, server.StatusInternal.Deployment.State)
	require.True(t, w.world.isPoweredOn(), "a failed deployment leaves the server running")
	require.Equal(t, []string{"system:1"}, w.world.mediaInserted(), "a failed deployment leaves the virtual media alone")

	// Run test
	err = w.service.DeploymentControlLoop(ctx, nil)

	// Assert
	require.NoError(t, err)

	after, err := w.repo.GetByName(ctx, worldServerName)
	require.NoError(t, err)
	require.Equal(t, deploymentStateSequence(server), deploymentStateSequence(*after), "a failed deployment is not picked up again")
}

// TestServerService_DeploymentControlLoopGivesUpOnAServerNeverReachingAState
// asserts, that a wait, whose trigger keeps being accepted while the server
// never reaches the state it asks for, is ended by the timeout of the deployment
// as a whole rather than repeating forever.
func TestServerService_DeploymentControlLoopGivesUpOnAServerNeverReachingAState(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
		worldOptions: []func(*bmcWorld){
			func(world *bmcWorld) { world.ignorePowerOffs = 1000 },
		},
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	server := driveDeployment(t, ctx, w, false)

	deployment := server.StatusInternal.Deployment

	require.Equal(t, api.ServerDeploymentStateFailed, deployment.State)
	require.Contains(t, deployment.LastError, "Deployment did not complete within "+config.ServerDeploymentTimeout.String())
	require.Equal(t, api.ServerStatusUnregistered, server.Status)
	require.Equal(t, api.ServerStatusDetailUnregisteredDeploymentFailed, server.StatusDetail)

	states := deploymentStateSequence(server)
	require.Equal(t, slices.Concat(deploymentStatesPreparing, deploymentStatesBIOSPass[:2]), states[:4])

	for _, state := range states[2 : len(states)-1] {
		require.Contains(
			t, deploymentStatesBIOSPass[:2], state,
			"the deployment fell back to the trigger of the wait it timed out in until it ran out of time",
		)
	}
}

// TestServerService_DeploymentControlLoopSurvivesAnUnobservableWait asserts,
// that a wait, whose condition can not be evaluated at all, is repeated until it
// can be, instead of failing the step.
func TestServerService_DeploymentControlLoopSurvivesAnUnobservableWait(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	blinded := 0

	server := driveDeployment(t, ctx, w, false, func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server) {
		t.Helper()

		if server.StatusInternal.Deployment.State != api.ServerDeploymentStateWaitPowerOffBIOS || blinded >= 2 {
			w.world.setGetDataFails(false)
			return
		}

		blinded++

		w.world.setGetDataFails(true)
	})

	require.Equal(t, 2, blinded, "the BMC refused to answer for two ticks")
	require.Equal(t, deploymentStatesHappyPath(), deploymentStateSequence(server))
	require.Zero(t, server.StatusInternal.Deployment.Retries, "a wait, that can not be evaluated, spends no retry")

	log.Contains("Failed to evaluate the deployment wait condition")(t, w.logBuf)
}
