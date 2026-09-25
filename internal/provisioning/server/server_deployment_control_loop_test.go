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
		name              string
		forceReboot       bool
		resolution        *provisioning.BIOSProfileResolution
		trackMedia        bool
		worldOptions      []func(*bmcWorld)
		request           func(request *provisioning.ServerDeploymentRequest)
		rebuildService    bool
		cancelAt          api.ServerDeploymentState
		cancelTwice       bool
		cancelSkipCleanup bool

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
				require.Zero(t, world.callCount("EnableSecureBoot"), "a deployment, that enrolls no certificates, leaves the state of secure boot to the operator")
				require.False(t, world.secureBootEnabled)
			},
		},
		{
			name:        "success - the secure boot certificates are enrolled from an enrollment media",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Zero(t, world.callCount("ApplySecureBootCertificates"), "the enrollment media replaces the enrollment through the BMC")
				require.True(t, world.secureBootEnabled, "secure boot is switched on once the certificates are enrolled")
				require.Equal(t, 1, world.callCount("ResetSecureBootKeys"), "the key databases are cleared to reach the setup mode")
				require.Equal(t, 1, world.callCount("GenerateSecureBootMedia"))
				require.Equal(t, worldSecureBootModeUser, world.secureBootMode, "the enrollment takes the server back out of the setup mode")
				require.Empty(t, world.mediaInserted(), "the clean up ejects the installation media")
			},
		},
		{
			name:        "success - the enrollment media is booted on a server, that is in setup mode already",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.secureBootMode = worldSecureBootModeSetup },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				[]api.ServerDeploymentState{api.ServerDeploymentStateResetSecureBootKeys},
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, worldSecureBootModeUser, world.secureBootMode, "the enrollment takes the server out of the setup mode")
			},
		},
		{
			name:        "success - the firmware brings the server back up after the power has been cut",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.relapsesAfterSecureBootReset = true },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				deploymentStatesSecureBootReset[4:],
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Server is powered on again after it had been reported powered off"),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, worldSecureBootModeUser, world.secureBootMode, "the wait goes back to the power off instead of taking the trough of the firmware reset for a settled power off, so the enrollment media is attached to a server, that is down")
			},
		},
		{
			name:        "success - the BMC rejects the enrollment media while the server is in its POST",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.attachMediaErrs = queue.Errs{domain.NewNotSettledErr(boom.Error)} },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				[]api.ServerDeploymentState{api.ServerDeploymentStateAttachSecureBootMedia},
				deploymentStatesSecureBootReset[4:],
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:           api.ServerStatusPending,
			wantStatusDetail:     api.ServerStatusDetailPendingRegistering,
			wantFallbackAttempts: 1,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, worldSecureBootModeUser, world.secureBootMode, "the enrollment media is attached once the server has settled")
			},
		},
		{
			name:        "success - the BMC rejects enabling secure boot while the server is in its POST",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.enableSecureBootErrs = queue.Errs{domain.NewNotSettledErr(boom.Error)} },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				[]api.ServerDeploymentState{api.ServerDeploymentStateEnableSecureBoot},
				deploymentStatesSecureBootMedia[4:],
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:           api.ServerStatusPending,
			wantStatusDetail:     api.ServerStatusDetailPendingRegistering,
			wantFallbackAttempts: 1,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.True(t, world.secureBootEnabled, "secure boot is switched on once the server has settled")
			},
		},
		{
			name:        "failure - the BMC keeps rejecting the enrollment media while the server is in its POST",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) {
					w.attachMediaErrs = queue.Errs{
						domain.NewNotSettledErr(boom.Error),
						domain.NewNotSettledErr(boom.Error),
						domain.NewNotSettledErr(boom.Error),
						domain.NewNotSettledErr(boom.Error),
					}
				},
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				[]api.ServerDeploymentState{api.ServerDeploymentStateAttachSecureBootMedia},
				deploymentStatesSecureBootReset[4:],
				[]api.ServerDeploymentState{api.ServerDeploymentStateAttachSecureBootMedia},
				deploymentStatesSecureBootReset[4:],
				[]api.ServerDeploymentState{api.ServerDeploymentStateAttachSecureBootMedia},
				deploymentStatesSecureBootReset[4:],
				[]api.ServerDeploymentState{
					api.ServerDeploymentStateAttachSecureBootMedia,
					api.ServerDeploymentStateFailed,
				},
			),
			wantStatus:           api.ServerStatusUnregistered,
			wantStatusDetail:     api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:      api.ServerDeploymentStateAttachSecureBootMedia,
			wantFallbackAttempts: config.ServerDeploymentStepRetries,
			wantLastError:        boom.Error.Error(),
		},
		{
			name:        "success - an allow listed signature is reported as lost with the enrollment media",
			forceReboot: true,
			resolution: func() *provisioning.BIOSProfileResolution {
				resolution := deploymentTestResolution()
				resolution.SecureBoot.DBX = api.BIOSSecureBootDatabase{
					Signatures: map[string]bool{"a-revoked-hash": true},
				}

				return resolution
			}(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("the enrollment media can only enroll certificates, so it is lost"),
		},
		{
			name:        "success - the server is only powered on once the BMC is done clearing the key databases",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.slowSecureBootReset = true },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBootReset,
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.False(t, world.poweredOnWhileResetting, "the server must not be powered on while the BMC is still clearing the key databases")
			},
		},
		{
			name:        "error - the BMC does not support the reset of the secure boot keys",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.noSecureBootReset = true },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				[]api.ServerDeploymentState{
					api.ServerDeploymentStateResetSecureBootKeys,
					api.ServerDeploymentStateFailed,
				},
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:  api.ServerDeploymentStateResetSecureBootKeys,
			wantLastError:    "Resetting the secure boot keys is not supported",
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
				deploymentStatesSecureBootSettle,
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

				require.Equal(t, 2, world.mediaResets, "the read progress is dropped when the media is attached and again when the deployment is cleaned up")

				// A server, that does not reboot on its own, is installed and
				// waiting for the media to go, so the idle period is the whole
				// latency of the only signal telling the deployment about it.
				require.LessOrEqual(
					t, world.mediaEjectedAfter(), deploymentMaxMediaEjectDelay,
					"the media is ejected right after the installer stopped reading it",
				)
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
			assertLog:        log.Contains("Installation completed, the installation media has been read and is idle"),
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
			name:       "failed - the BMC uploads the installation media, while only its read progress could tell",
			resolution: deploymentTestResolution(),
			trackMedia: true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.uploadTransfer = true },
			},
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.Force = true
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall[:2],
				[]api.ServerDeploymentState{api.ServerDeploymentStateFailed},
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:  api.ServerDeploymentStateWaitMediaAttached,
			wantLastError:    "uploads the installation media instead of streaming it",
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, []string{"system:1"}, world.mediaInserted(), "the failed deployment leaves the installation media attached, the way every failure does")
			},
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
				func(w *bmcWorld) {
					w.installDuration = config.ServerDeploymentMinInstallDuration / 2
					w.bootsMediaAgain = true
				},
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
			name:        "success - the BMC caches the installation media instead of streaming it",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			trackMedia:  true,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.cachesMedia = true },
			},

			wantStates:       deploymentStatesHappyPath(),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.NotContains("Installation completed, the installation media has been read and is idle"),
		},
		{
			name:        "success - the firmware reboots before the installer even started",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.rebootsEarly = true; w.postDuration = worldEarlyRebootDelay + worldBootDuration },
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

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize[:3],
				[]api.ServerDeploymentState{api.ServerDeploymentStatePowerOnReboot},
				deploymentStatesFinalize[2:],
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("Deployment wait reverted"),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.True(t, world.isPoweredOn(), "a server, that stayed off after the installation, is powered on again")
			},
		},
		{
			// The wait does not cut the power again, while the server has not been
			// seen powered off, so a BMC ignoring the power off costs a timeout per
			// attempt.
			name:        "success - a wait times out and falls back to its trigger",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.ignorePowerOffFor = worldIgnorePowerOffDuration },
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass[:2],
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
			name:        "success - a BIOS attribute reflecting the secure boot state is verified after the enrollment",
			forceReboot: true,
			resolution: func() *provisioning.BIOSProfileResolution {
				resolution := deploymentTestResolution()
				resolution.Attributes["SecureBootStatus"] = "Enabled"

				return resolution
			}(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) {
					w.secureBootMode = worldSecureBootModeSetup
					w.biosSecureBootStatusAttribute = "SecureBootStatus"
				},
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				[]api.ServerDeploymentState{api.ServerDeploymentStateResetSecureBootKeys},
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesFinalize,
			),
			wantStatus:       api.ServerStatusPending,
			wantStatusDetail: api.ServerStatusDetailPendingRegistering,
			assertLog:        log.Contains("verifying them again once the secure boot certificates are enrolled"),
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.Equal(t, worldSecureBootModeUser, world.secureBootMode, "the enrollment takes the server out of the setup mode")
				require.Equal(t, 2, world.callCount("ApplyBIOSAttributes"), "one apply per BIOS pass, no pass is repeated for an attribute, that only the enrollment can satisfy")
			},
		},
		{
			name:        "failure - a BIOS attribute does not hold even after the secure boot certificates are enrolled",
			forceReboot: true,
			resolution: func() *provisioning.BIOSProfileResolution {
				resolution := deploymentTestResolution()
				resolution.Attributes["SecureBootStatus"] = "Enabled"

				return resolution
			}(),
			request: func(request *provisioning.ServerDeploymentRequest) {
				request.SecureBootEnrollmentMedia = true
			},
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) {
					w.secureBootMode = worldSecureBootModeSetup
					w.biosSecureBootStatusAttribute = "SecureBootStatus"
					w.biosSecureBootStatusStuck = true
				},
			},

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				[]api.ServerDeploymentState{api.ServerDeploymentStateResetSecureBootKeys},
				deploymentStatesSecureBootMedia,
				deploymentStatesMediaCleared,
				[]api.ServerDeploymentState{
					api.ServerDeploymentStateEnableSecureBoot,
					api.ServerDeploymentStatePowerOnSecureBoot,
					api.ServerDeploymentStateWaitSecureBootSettled,
					api.ServerDeploymentStateFailed,
				},
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentFailed,
			wantFailedState:  api.ServerDeploymentStateWaitSecureBootSettled,
			wantLastError:    "not even after the secure boot certificates were enrolled",
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
		{
			name:              "cancelled - skipping the clean up",
			forceReboot:       true,
			resolution:        deploymentTestResolution(),
			cancelAt:          api.ServerDeploymentStateWaitInstall,
			cancelSkipCleanup: true,

			wantStates: slices.Concat(
				deploymentStatesPreparing,
				deploymentStatesBIOSPass,
				deploymentStatesBIOSDeferredPass,
				deploymentStatesSecureBootOff,
				deploymentStatesSecureBoot,
				deploymentStatesMediaCleared,
				deploymentStatesSecureBootSettle,
				deploymentStatesInstall,
				deploymentStatesCancelSkipCleanup,
			),
			wantStatus:       api.ServerStatusUnregistered,
			wantStatusDetail: api.ServerStatusDetailUnregisteredDeploymentCancelled,
			assertWorld: func(t *testing.T, world *bmcWorld) {
				t.Helper()

				require.NotEmpty(t, world.mediaInserted(), "the clean up is skipped, so the installation media stays attached")
				require.True(t, world.isPoweredOn(), "the clean up is skipped, so the server is left running")
			},
		},
		{
			name:        "cancelled - the BMC completes the ejection after the power off",
			forceReboot: true,
			resolution:  deploymentTestResolution(),
			cancelAt:    api.ServerDeploymentStateWaitInstall,
			worldOptions: []func(*bmcWorld){
				func(w *bmcWorld) { w.ejectDelay = worldEjectDelay },
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

				require.Empty(
					t, world.mediaInserted(),
					"a cancelled deployment waits for the ejection it issued, instead of completing on the power state alone",
				)
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

					require.NoError(t, svc.CancelDeploymentByName(ctx, worldServerName, tc.cancelSkipCleanup))

					if tc.cancelTwice {
						require.NoError(t, svc.CancelDeploymentByName(ctx, worldServerName, tc.cancelSkipCleanup), "cancelling twice is a no-op")
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

// TestServerService_DeploymentControlLoopKeepsTheSecureBootSettleBootAcrossARetry
// drives the deployment through the enrollment of the secure boot certificates
// and then rewinds it into the enrollment, the way a crash between the write and
// the transition, that records it, leaves the deployment behind. The BMC reports
// the key databases as applied by then, so the re-issued enrollment writes
// nothing, while the firmware still has the certificates to pick up: the settle
// boot has to run regardless.
func TestServerService_DeploymentControlLoopKeepsTheSecureBootSettleBootAcrossARetry(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	for range deploymentDriveIterations {
		if w.world.callCount("ApplySecureBootCertificates") > 0 {
			break
		}

		server, err := w.repo.GetByName(ctx, worldServerName)
		require.NoError(t, err)

		before := server.StatusInternal.Deployment.State

		require.NoError(t, w.world.settle(ctx))
		require.NoError(t, w.service.DeploymentControlLoop(ctx, nil))

		after, err := w.repo.GetByName(ctx, worldServerName)
		require.NoError(t, err)

		advance := deploymentTick
		if after.StatusInternal.Deployment.State == before {
			advance = deploymentIdleTick
		}

		w.clock.advance(advance)
	}

	require.Equal(t, 1, w.world.callCount("ApplySecureBootCertificates"), "the enrollment has run once")

	crashed, err := w.repo.GetByName(ctx, worldServerName)
	require.NoError(t, err)

	deployment := crashed.StatusInternal.Deployment
	require.True(t, deployment.SecureBootAttempted, "the attempt is recorded before the enrollment writes anything")

	// Everything the enrollment produced is gone, only what was persisted before
	// it ran is left, and the deployment is back in the trigger state.
	deployment.State = api.ServerDeploymentStateSecureBoot
	deployment.SecureBootPending = false

	require.NoError(t, w.repo.Update(ctx, *crashed))

	// The key databases hold the certificates now, so the enrollment leaves them
	// untouched and reports, that it wrote nothing.
	w.world.mu.Lock()
	w.world.secureBootEnrolls = false
	w.world.mu.Unlock()

	server := driveDeployment(t, ctx, w, false)

	require.Equal(t, 2, w.world.callCount("ApplySecureBootCertificates"), "the enrollment is re-issued")

	states := deploymentStateSequence(server)

	retried := slices.Index(states, api.ServerDeploymentStateSecureBoot)
	require.NotEqual(t, -1, retried)

	retried = slices.Index(states[retried+1:], api.ServerDeploymentStateSecureBoot) + retried + 1

	require.Equal(
		t,
		slices.Concat(
			deploymentStatesMediaCleared,
			deploymentStatesSecureBootSettle,
			deploymentStatesInstall,
			deploymentStatesFinalize,
		),
		states[retried+1:],
		"the re-issued enrollment keeps the settle boot, since an earlier attempt may have written the key databases",
	)

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
// never reaches the state it asks for, is ended by the retry budget of the wait
// rather than repeating until the deployment as a whole times out.
func TestServerService_DeploymentControlLoopGivesUpOnAServerNeverReachingAState(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
		worldOptions: []func(*bmcWorld){
			func(world *bmcWorld) { world.ignorePowerOffFor = 10 * config.ServerDeploymentTimeout },
		},
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	server := driveDeployment(t, ctx, w, false)

	deployment := server.StatusInternal.Deployment

	require.Equal(t, api.ServerDeploymentStateFailed, deployment.State)
	require.Contains(t, deployment.LastError, `The deployment step "wait-power-off-bios" did not complete within `+config.ServerDeploymentStepTimeout.String())
	require.Equal(t, config.ServerDeploymentStepRetries, deployment.WaitRetries, "the wait fell back to its trigger until its retry budget was spent")
	require.Equal(t, api.ServerStatusUnregistered, server.Status)
	require.Equal(t, api.ServerStatusDetailUnregisteredDeploymentFailed, server.StatusDetail)

	states := deploymentStateSequence(server)
	require.Equal(t, slices.Concat(deploymentStatesPreparing, deploymentStatesBIOSPass[:2]), states[:4])
	require.Len(t, states, 4+2*config.ServerDeploymentStepRetries+1, "every fallback repeats the trigger and the wait once")

	for _, state := range states[2 : len(states)-1] {
		require.Contains(
			t, deploymentStatesBIOSPass[:2], state,
			"the deployment fell back to the trigger of the wait it timed out in until its retry budget was spent",
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

func TestServerService_DeploymentControlLoopDoesNotClearMediaItCouldNotSee(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
	})

	require.Equal(t, []string{"system:1"}, w.world.mediaInserted(), "the server holds the media of a previous run")

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	server := driveDeployment(t, ctx, w, false, func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server) {
		t.Helper()

		// The BMC stops answering for its virtual media devices while it settles
		// after the BIOS work and is still doing so a few states later, when the
		// deployment goes to clear the media. A tick advances several states, so
		// the outage is started from the last wait before that run.
		if server.StatusInternal.Deployment.State != api.ServerDeploymentStateWaitBIOSAppliedDeferred {
			return
		}

		w.world.setPartUnavailable(api.BMCDataPartVirtualMedia, "BMC returned HTTP 503: iDRAC is currently unable to display any information")
	})

	deployment := server.StatusInternal.Deployment

	states := deploymentStateSequence(server)
	require.Contains(t, states, api.ServerDeploymentStateClearMedia)
	require.NotContains(
		t, states, api.ServerDeploymentStateWaitMediaCleared,
		"the media was never cleared, so the wait after it must not have been entered",
	)
	require.NotContains(
		t, states, api.ServerDeploymentStateAttachMedia,
		"the deployment must not attach its media on top of the one it could not clear",
	)

	require.Equal(t, api.ServerDeploymentStateFailed, deployment.State)
	require.Contains(t, deployment.LastError, api.BMCDataPartVirtualMedia.String())

	require.Equal(
		t, []string{"system:1"}, w.world.mediaInserted(),
		"the media of the previous run is still attached, which is exactly why the deployment stopped",
	)
}

func TestServerService_DeploymentControlLoopClearsTheMediaOnceTheBMCAnswersAgain(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	unavailable := 0

	server := driveDeployment(t, ctx, w, false, func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server) {
		t.Helper()

		// The outage lasts until clearing the media has failed on it once, so the
		// deployment is known to have stopped rather than passed through.
		if server.StatusInternal.Deployment.Retries > 0 {
			w.world.setPartAvailable(api.BMCDataPartVirtualMedia)
			return
		}

		if server.StatusInternal.Deployment.State != api.ServerDeploymentStateWaitBIOSAppliedDeferred {
			return
		}

		unavailable++

		w.world.setPartUnavailable(api.BMCDataPartVirtualMedia, "BMC returned HTTP 503: iDRAC is currently unable to display any information")
	})

	require.NotZero(t, unavailable, "the BMC did not report its virtual media devices for a while")
	require.Equal(t, api.ServerDeploymentStateCompleted, server.StatusInternal.Deployment.State)
	require.Equal(t, deploymentStatesHappyPath(), deploymentStateSequence(server))
	require.Empty(t, w.world.mediaInserted(), "the media of the previous run has been ejected after all")
}

func TestServerService_DeploymentControlLoopDoesNotDetachMediaItCouldNotSee(t *testing.T) {
	ctx := t.Context()

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
		worldOptions: []func(*bmcWorld){
			func(world *bmcWorld) { world.mediaEjectDelay = time.Hour },
		},
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	server := driveDeployment(t, ctx, w, false, func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server) {
		t.Helper()

		if server.StatusInternal.Deployment.State != api.ServerDeploymentStateDetachMedia &&
			server.StatusInternal.Deployment.State != api.ServerDeploymentStateWaitMediaDetached {
			return
		}

		w.world.setPartUnavailable(api.BMCDataPartVirtualMedia, "BMC returned HTTP 503: iDRAC is currently unable to display any information")
	})

	deployment := server.StatusInternal.Deployment

	require.Equal(t, api.ServerDeploymentStateFailed, deployment.State)
	require.NotContains(
		t, deploymentStateSequence(server), api.ServerDeploymentStateWaitReboot,
		"a media list, that could not be collected, does not establish the media is gone",
	)
}
