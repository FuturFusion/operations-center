package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

var deploymentTestNow = time.Date(2026, 3, 12, 10, 57, 43, 0, time.UTC)

func Test_deploymentNextState(t *testing.T) {
	tests := []struct {
		name       string
		deployment provisioning.ServerDeployment
		next       api.ServerDeploymentState

		want api.ServerDeploymentState
	}{
		{
			name:       "state without a skip",
			deployment: provisioning.ServerDeployment{},
			next:       api.ServerDeploymentStateAttachMedia,

			want: api.ServerDeploymentStateAttachMedia,
		},
		{
			name:       "first BIOS pass is pending",
			deployment: provisioning.ServerDeployment{BIOSPending: true},
			next:       api.ServerDeploymentStatePowerOffBIOS,

			want: api.ServerDeploymentStatePowerOffBIOS,
		},
		{
			name:       "first BIOS pass is passed by",
			deployment: provisioning.ServerDeployment{BIOSDeferredPending: true},
			next:       api.ServerDeploymentStatePowerOffBIOS,

			want: api.ServerDeploymentStatePowerOffBIOSDeferred,
		},
		{
			name:       "deferred BIOS pass is pending",
			deployment: provisioning.ServerDeployment{BIOSDeferredPending: true},
			next:       api.ServerDeploymentStatePowerOffBIOSDeferred,

			want: api.ServerDeploymentStatePowerOffBIOSDeferred,
		},
		{
			name:       "both BIOS passes are passed by",
			deployment: provisioning.ServerDeployment{},
			next:       api.ServerDeploymentStatePowerOffBIOS,

			want: api.ServerDeploymentStatePowerOffSecureBoot,
		},
		{
			name:       "secure boot enrollment is requested",
			deployment: provisioning.ServerDeployment{},
			next:       api.ServerDeploymentStateSecureBoot,

			want: api.ServerDeploymentStateSecureBoot,
		},
		{
			name: "secure boot enrollment is skipped",
			deployment: provisioning.ServerDeployment{
				Request: provisioning.ServerDeploymentRequest{SkipSecureBootCertificates: true},
			},
			next: api.ServerDeploymentStateSecureBoot,

			want: api.ServerDeploymentStateClearMedia,
		},
		{
			name:       "firmware has certificates to pick up",
			deployment: provisioning.ServerDeployment{SecureBootPending: true},
			next:       api.ServerDeploymentStatePowerOnSecureBoot,

			want: api.ServerDeploymentStatePowerOnSecureBoot,
		},
		{
			name:       "firmware has nothing to pick up",
			deployment: provisioning.ServerDeployment{},
			next:       api.ServerDeploymentStatePowerOnSecureBoot,

			want: api.ServerDeploymentStateAttachMedia,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentNextState(&tc.deployment, tc.next)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentIsBIOSDeferredPass(t *testing.T) {
	tests := []struct {
		name  string
		state api.ServerDeploymentState

		want bool
	}{
		{name: "power off", state: api.ServerDeploymentStatePowerOffBIOSDeferred, want: true},
		{name: "wait power off", state: api.ServerDeploymentStateWaitPowerOffBIOSDeferred, want: true},
		{name: "apply", state: api.ServerDeploymentStateApplyBIOSDeferred, want: true},
		{name: "power on", state: api.ServerDeploymentStatePowerOnBIOSDeferred, want: true},
		{name: "wait applied", state: api.ServerDeploymentStateWaitBIOSAppliedDeferred, want: true},
		{name: "verify", state: api.ServerDeploymentStateVerifyBIOSDeferred, want: true},
		{name: "first pass", state: api.ServerDeploymentStateApplyBIOS, want: false},
		{name: "unrelated state", state: api.ServerDeploymentStateAttachMedia, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentIsBIOSDeferredPass(tc.state)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentBackoff(t *testing.T) {
	tests := []struct {
		name    string
		retries int

		want time.Duration
	}{
		{name: "negative", retries: -1, want: 0},
		{name: "no attempt spent yet", retries: 0, want: 0},
		{name: "first retry", retries: 1, want: config.ServerDeploymentRetryBackoff},
		{name: "second retry", retries: 2, want: 2 * config.ServerDeploymentRetryBackoff},
		{name: "third retry", retries: 3, want: 4 * config.ServerDeploymentRetryBackoff},
		{name: "capped", retries: 10, want: config.ServerDeploymentRetryBackoffMax},
		{name: "capped beyond the shift limit", retries: 64, want: config.ServerDeploymentRetryBackoffMax},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentBackoff(tc.retries)

			require.Equal(t, tc.want, got)
			require.LessOrEqual(t, got, config.ServerDeploymentRetryBackoffMax, "the backoff never exceeds its upper limit")
		})
	}
}

func Test_deploymentStateDefinition_callTimeoutOrDefault(t *testing.T) {
	tests := []struct {
		name  string
		state api.ServerDeploymentState

		want time.Duration
	}{
		{
			name:  "state without a budget of its own",
			state: api.ServerDeploymentStateRefreshBMCData,
			want:  config.ServerDeploymentStepCallTimeout,
		},
		{
			name:  "attaching the installation media",
			state: api.ServerDeploymentStateAttachMedia,
			want:  config.ServerDeploymentAttachMediaCallTimeout,
		},
		{
			name:  "enrolling the secure boot certificates",
			state: api.ServerDeploymentStateSecureBoot,
			want:  config.ServerDeploymentSecureBootCallTimeout,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deploymentStates[tc.state].callTimeoutOrDefault())
		})
	}
}

func Test_deploymentStates_callTimeoutIsAlwaysPositive(t *testing.T) {
	for state, definition := range deploymentStates {
		require.Positive(t, definition.callTimeoutOrDefault(), "state %q leaves the BMC operations of an attempt unbounded", state)
	}
}

func Test_deploymentInstallRebootEndsTheWait(t *testing.T) {
	readOut := provisioning.SeedImageProgress{
		Size:         4 * config.ServerDeploymentMediaMinBytesRead,
		BytesCovered: config.ServerDeploymentMediaMinBytesRead,
	}

	barelyRead := provisioning.SeedImageProgress{
		Size:         4 * config.ServerDeploymentMediaMinBytesRead,
		BytesCovered: config.ServerDeploymentMediaMinBytesRead - 1,
	}

	tests := []struct {
		name          string
		installingFor time.Duration
		progress      provisioning.SeedImageProgress
		progressKnown bool

		want bool
	}{
		{
			name:          "media read out before the installation could be done",
			installingFor: time.Minute,
			progress:      readOut,
			progressKnown: true,

			want: true,
		},
		{
			name:          "media barely read before the installation could be done",
			installingFor: time.Minute,
			progress:      barelyRead,
			progressKnown: true,

			want: false,
		},
		{
			name:          "read progress can not be told before the installation could be done",
			installingFor: time.Minute,
			progress:      readOut,
			progressKnown: false,

			want: false,
		},
		{
			name:          "read progress can not be told once the installation could be done",
			installingFor: config.ServerDeploymentMinInstallDuration,
			progress:      provisioning.SeedImageProgress{},
			progressKnown: false,

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deployment := &provisioning.ServerDeployment{
				StateEnteredAt: deploymentTestNow,
			}

			got := deploymentInstallRebootEndsTheWait(deploymentTestNow.Add(tc.installingFor), deployment, tc.progress, tc.progressKnown)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_biosAttributeMatches(t *testing.T) {
	tests := []struct {
		name         string
		currentValue any
		want         any

		wantMatch bool
	}{
		{name: "equal strings", currentValue: "Enabled", want: "Enabled", wantMatch: true},
		{name: "different case", currentValue: "enabled", want: "Enabled", wantMatch: true},
		{name: "surrounding whitespace", currentValue: "  Enabled ", want: "Enabled", wantMatch: true},
		{name: "number reported as a string", currentValue: "4", want: 4, wantMatch: true},
		{name: "boolean reported as a string", currentValue: "True", want: true, wantMatch: true},
		{name: "different values", currentValue: "Disabled", want: "Enabled", wantMatch: false},
		{name: "unset current value", currentValue: nil, want: "Enabled", wantMatch: false},
		{name: "both unset", currentValue: nil, want: nil, wantMatch: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := biosAttributeMatches(api.BIOSAttribute{Name: "one", CurrentValue: tc.currentValue}, tc.want)

			require.Equal(t, tc.wantMatch, got)
		})
	}
}

func Test_deploymentBIOSPassPending(t *testing.T) {
	current := map[string]api.BIOSAttribute{
		"one": {Name: "one", CurrentValue: "Enabled"},
		"two": {Name: "two", CurrentValue: 4},
	}

	tests := []struct {
		name       string
		attributes map[string]any

		want bool
	}{
		{name: "nothing to apply", attributes: nil, want: false},
		{name: "all attributes are applied", attributes: map[string]any{"one": "Enabled", "two": "4"}, want: false},
		{name: "an attribute mismatches", attributes: map[string]any{"one": "Disabled"}, want: true},
		{name: "an attribute is not reported", attributes: map[string]any{"three": "Enabled"}, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentBIOSPassPending(current, tc.attributes)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentBIOSAttributes(t *testing.T) {
	deployment := provisioning.ServerDeployment{
		BIOSAttributes:         map[string]any{"one": "Enabled"},
		BIOSDeferredAttributes: map[string]any{"two": "Disabled"},
	}

	tests := []struct {
		name  string
		state api.ServerDeploymentState

		want map[string]any
	}{
		{name: "first pass", state: api.ServerDeploymentStateApplyBIOS, want: deployment.BIOSAttributes},
		{name: "deferred pass", state: api.ServerDeploymentStateApplyBIOSDeferred, want: deployment.BIOSDeferredAttributes},
		{name: "verification of the deferred pass", state: api.ServerDeploymentStateVerifyBIOSDeferred, want: deployment.BIOSDeferredAttributes},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deployment.State = tc.state

			got := deploymentBIOSAttributes(&deployment)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentMediaBytesRequired(t *testing.T) {
	tests := []struct {
		name string
		size int64

		want int64
	}{
		{name: "unknown size", size: 0, want: config.ServerDeploymentMediaMinBytesRead},
		{name: "negative size", size: -1, want: config.ServerDeploymentMediaMinBytesRead},
		{name: "media smaller than the minimum", size: 1024, want: 1024},
		{name: "media larger than the minimum", size: 4 * config.ServerDeploymentMediaMinBytesRead, want: config.ServerDeploymentMediaMinBytesRead},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentMediaBytesRequired(tc.size)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentMediaReadOut(t *testing.T) {
	tests := []struct {
		name     string
		progress provisioning.SeedImageProgress

		want bool
	}{
		{
			name:     "nothing read",
			progress: provisioning.SeedImageProgress{Size: 4 * config.ServerDeploymentMediaMinBytesRead},

			want: false,
		},
		{
			name: "enough read",
			progress: provisioning.SeedImageProgress{
				Size:         4 * config.ServerDeploymentMediaMinBytesRead,
				BytesCovered: config.ServerDeploymentMediaMinBytesRead,
			},

			want: true,
		},
		{
			name: "a small media read completely",
			progress: provisioning.SeedImageProgress{
				Size:         1024,
				BytesCovered: 1024,
			},

			want: true,
		},
		{
			name: "distinct bytes fall short of the bytes served",
			progress: provisioning.SeedImageProgress{
				Size:         4 * config.ServerDeploymentMediaMinBytesRead,
				BytesServed:  8 * config.ServerDeploymentMediaMinBytesRead,
				BytesCovered: 1024,
			},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentMediaReadOut(tc.progress)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentMediaIdle(t *testing.T) {
	tests := []struct {
		name     string
		progress provisioning.SeedImageProgress

		want bool
	}{
		{
			name:     "nothing read at all",
			progress: provisioning.SeedImageProgress{},

			want: false,
		},
		{
			name:     "read just now",
			progress: provisioning.SeedImageProgress{LastRead: deploymentTestNow},

			want: false,
		},
		{
			name:     "idle for less than the idle period",
			progress: provisioning.SeedImageProgress{LastRead: deploymentTestNow.Add(-config.ServerDeploymentMediaIdlePeriod + time.Second)},

			want: false,
		},
		{
			name:     "idle for the idle period",
			progress: provisioning.SeedImageProgress{LastRead: deploymentTestNow.Add(-config.ServerDeploymentMediaIdlePeriod)},

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentMediaIdle(deploymentTestNow, tc.progress)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentInstallCouldBeDone(t *testing.T) {
	tests := []struct {
		name           string
		stateEnteredAt time.Time

		want bool
	}{
		{
			name:           "just entered",
			stateEnteredAt: deploymentTestNow,

			want: false,
		},
		{
			name:           "shortly before the minimum install duration",
			stateEnteredAt: deploymentTestNow.Add(-config.ServerDeploymentMinInstallDuration + time.Second),

			want: false,
		},
		{
			name:           "at the minimum install duration",
			stateEnteredAt: deploymentTestNow.Add(-config.ServerDeploymentMinInstallDuration),

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentInstallCouldBeDone(deploymentTestNow, &provisioning.ServerDeployment{StateEnteredAt: tc.stateEnteredAt})

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentRebootObserved(t *testing.T) {
	snapshotTaken := deploymentTestNow.Add(-config.ServerDeploymentRebootObservationWindow)

	rebootedSnapshot := provisioning.ServerDeploymentBMCSnapshot{
		Taken:         snapshotTaken,
		LastResetTime: snapshotTaken.Add(-time.Hour),
		BootProgress:  api.BMCBootProgress{LastState: "OSRunning", LastStateTime: snapshotTaken.Add(-time.Hour)},
	}

	tests := []struct {
		name           string
		snapshot       provisioning.ServerDeploymentBMCSnapshot
		stateEnteredAt time.Time
		current        api.BMCData

		wantRebooted bool
		wantObserved bool
	}{
		{
			name:           "no snapshot has been taken",
			snapshot:       provisioning.ServerDeploymentBMCSnapshot{},
			stateEnteredAt: deploymentTestNow,

			wantRebooted: true,
			wantObserved: false,
		},
		{
			name:           "the reset time advanced",
			snapshot:       rebootedSnapshot,
			stateEnteredAt: deploymentTestNow,
			current:        api.BMCData{ServerLastResetTime: deploymentTestNow},

			wantRebooted: true,
			wantObserved: true,
		},
		{
			name:           "the boot progress regressed",
			snapshot:       rebootedSnapshot,
			stateEnteredAt: deploymentTestNow,
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{LastState: "MemoryInitializationStarted", LastStateTime: deploymentTestNow},
			},

			wantRebooted: true,
			wantObserved: true,
		},
		{
			name:           "no reboot within the observation window",
			snapshot:       rebootedSnapshot,
			stateEnteredAt: deploymentTestNow,
			current:        api.BMCData{},

			wantRebooted: false,
			wantObserved: false,
		},
		{
			name:           "no reboot after the observation window",
			snapshot:       rebootedSnapshot,
			stateEnteredAt: deploymentTestNow.Add(-config.ServerDeploymentRebootObservationWindow),
			current:        api.BMCData{},

			wantRebooted: true,
			wantObserved: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deployment := provisioning.ServerDeployment{
				InstallSnapshot: tc.snapshot,
				StateEnteredAt:  tc.stateEnteredAt,
			}

			rebooted, observed := deploymentRebootObserved(deploymentTestNow, &deployment, tc.current)

			require.Equal(t, tc.wantRebooted, rebooted)
			require.Equal(t, tc.wantObserved, observed)
		})
	}
}

func Test_serverHasRegistered(t *testing.T) {
	tests := []struct {
		name   string
		status api.ServerStatus

		want bool
	}{
		{name: "pending", status: api.ServerStatusPending, want: true},
		{name: "ready", status: api.ServerStatusReady, want: true},
		{name: "unregistered", status: api.ServerStatusUnregistered, want: false},
		{name: "deploying", status: api.ServerStatusDeploying, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := serverHasRegistered(provisioning.Server{Status: tc.status})

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_deploymentSettleSnapshot(t *testing.T) {
	current := api.BMCData{
		ServerPowerState:    bmcPowerStateOn,
		ServerLastResetTime: deploymentTestNow.Add(-time.Hour),
		ServerBootProgress:  api.BMCBootProgress{LastState: "OSRunning", LastStateTime: deploymentTestNow.Add(-time.Hour)},
	}

	tests := []struct {
		name           string
		stateEnteredAt time.Time
		powerState     string

		wantSnapshot bool
	}{
		{
			name:           "the server is still powered off",
			stateEnteredAt: deploymentTestNow.Add(-config.ServerDeploymentSettleDelay),
			powerState:     bmcPowerStateOff,

			wantSnapshot: false,
		},
		{
			name:           "the settle delay has not passed yet",
			stateEnteredAt: deploymentTestNow,
			powerState:     bmcPowerStateOn,

			wantSnapshot: false,
		},
		{
			name:           "the server has settled",
			stateEnteredAt: deploymentTestNow.Add(-config.ServerDeploymentSettleDelay),
			powerState:     bmcPowerStateOn,

			wantSnapshot: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := current
			data.ServerPowerState = tc.powerState

			deployment := provisioning.ServerDeployment{StateEnteredAt: tc.stateEnteredAt}

			mutate := deploymentSettleSnapshot(deploymentTestNow, &deployment, data, func(deployment *provisioning.ServerDeployment, snapshot provisioning.ServerDeploymentBMCSnapshot) {
				deployment.InstallSnapshot = snapshot
			})

			if !tc.wantSnapshot {
				require.Nil(t, mutate)
				return
			}

			require.NotNil(t, mutate)

			mutate(&deployment)

			require.Equal(t, provisioning.NewServerDeploymentBMCSnapshot(deploymentTestNow, data), deployment.InstallSnapshot)
		})
	}
}

func Test_bmcWaitConditions(t *testing.T) {
	deployment := provisioning.ServerDeployment{
		Request:  provisioning.ServerDeploymentRequest{VirtualMediaID: "system:1"},
		MediaURL: "https://oc.example.com:8443/one.iso",
	}

	tests := []struct {
		name  string
		state api.ServerDeploymentState
		data  api.BMCData

		want bool
	}{
		{
			name:  "power is off",
			state: api.ServerDeploymentStateWaitPowerOffBIOS,
			data:  api.BMCData{ServerPowerState: bmcPowerStateOff},

			want: true,
		},
		{
			name:  "power is still on",
			state: api.ServerDeploymentStateWaitCancel,
			data:  api.BMCData{ServerPowerState: bmcPowerStateOn},

			want: false,
		},
		{
			name:  "no media is inserted",
			state: api.ServerDeploymentStateWaitMediaCleared,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1"},
			}},

			want: true,
		},
		{
			name:  "another media is still inserted",
			state: api.ServerDeploymentStateWaitMediaCleared,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", Inserted: true},
			}},

			want: false,
		},
		{
			name:  "the media holds the image",
			state: api.ServerDeploymentStateWaitMediaAttached,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/one.iso"},
			}},

			want: true,
		},
		{
			name:  "the media holds another image",
			state: api.ServerDeploymentStateWaitMediaAttached,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/other.iso"},
			}},

			want: false,
		},
		{
			name:  "the media is ejected",
			state: api.ServerDeploymentStateWaitMediaDetached,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1"},
			}},

			want: true,
		},
		{
			name:  "the media device is gone",
			state: api.ServerDeploymentStateWaitMediaDetached,
			data:  api.BMCData{},

			want: true,
		},
		{
			name:  "the media is still inserted",
			state: api.ServerDeploymentStateWaitMediaDetached,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/one.iso"},
			}},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			condition, ok := bmcWaitConditions[tc.state]
			require.True(t, ok, "state %q has no BMC wait condition", tc.state)

			require.Equal(t, tc.want, condition(&deployment, tc.data))
		})
	}
}

func Test_selectVirtualMediaID(t *testing.T) {
	tests := []struct {
		name string
		data api.BMCData

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "the only device",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
			}},

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "the only optical device",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"CD", "DVD"}},
			}},

			want:      "system:1",
			assertErr: require.NoError,
		},
		{
			name: "no device at all",
			data: api.BMCData{},

			assertErr: errassert.NotFoundError,
		},
		{
			name: "several optical devices",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", MediaTypes: []string{"CD"}},
				"system:2": {ID: "system:2", MediaTypes: []string{"DVD"}},
			}},

			want:      "system:1",
			assertErr: require.NoError,
		},
		{
			name: "several devices, none of them optical",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"manager:2": {ID: "manager:2"},
			}},

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "the optical device of the system wins over the one of the manager",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"CD", "DVD"}},
				"system:2":  {ID: "system:2", MediaTypes: []string{"CD", "DVD"}},
			}},

			want:      "system:2",
			assertErr: require.NoError,
		},
		{
			name: "the optical device of the manager wins over the non optical one of the system",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"CD", "DVD"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"USBStick"}},
			}},

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "no optical device at all, the one of the system wins",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"system:1":  {ID: "system:1"},
			}},

			want:      "system:1",
			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectVirtualMediaID(tc.data)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_describeVirtualMedia(t *testing.T) {
	tests := []struct {
		name string
		data api.BMCData

		want string
	}{
		{
			name: "no device",
			data: api.BMCData{},

			want: "no virtual media device",
		},
		{
			name: "the devices in a stable order",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1":  {ID: "system:1"},
				"manager:1": {ID: "manager:1"},
			}},

			want: "manager:1, system:1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, describeVirtualMedia(tc.data))
		})
	}
}

func Test_deploymentRetryFromError(t *testing.T) {
	wrapped := domain.NewRetryableErr(boom.Error)

	err := error(deploymentRetryFromError{
		state: api.ServerDeploymentStatePowerOffBIOS,
		err:   wrapped,
	})

	require.Equal(t, wrapped.Error(), err.Error(), "the sentinel reports the message of the error it carries")
	require.ErrorIs(t, err, boom.Error, "the wrapped error stays inspectable")
	require.True(t, domain.IsRetryableError(err), "the retryable marker survives the wrapping")

	retryFrom, ok := errors.AsType[deploymentRetryFromError](fmt.Errorf("wrapped: %w", err))
	require.True(t, ok, "the sentinel is found through another wrapping")
	require.Equal(t, api.ServerDeploymentStatePowerOffBIOS, retryFrom.state)

	_, ok = errors.AsType[deploymentRetryFromError](boom.Error)
	require.False(t, ok, "a plain error carries no state to go back to")
}

// Test_deploymentStates asserts the invariants of the deployment state machine,
// which the dispatcher relies on but can not check itself.
func Test_deploymentStates(t *testing.T) {
	for state, definition := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			var unmarshalled api.ServerDeploymentState
			require.NoError(t, unmarshalled.UnmarshalText([]byte(state)), "state %q is not a known deployment state", state)

			if definition.kind == deploymentStateKindTerminal {
				require.True(t, state.IsTerminal(), "terminal state %q does not report itself as terminal", state)
				require.Empty(t, definition.next, "terminal state %q leads somewhere", state)
				require.Empty(t, definition.fallback, "terminal state %q has a fallback", state)
				require.Zero(t, definition.timeout, "terminal state %q has a timeout", state)

				return
			}

			require.False(t, state.IsTerminal(), "non terminal state %q reports itself as terminal", state)
			require.NotEmpty(t, definition.detail, "state %q reports no status detail", state)
			require.NotEqual(t, state, definition.next, "state %q leads to itself", state)
			require.Contains(t, deploymentStates, definition.next, "state %q leads to the unknown state %q", state, definition.next)

			if definition.kind == deploymentStateKindAction {
				require.Empty(t, definition.fallback, "action state %q has a fallback", state)
				require.Zero(t, definition.timeout, "action state %q has a timeout", state)

				return
			}

			require.NotZero(t, definition.timeout, "wait state %q is not bounded by a timeout", state)

			if definition.fallback == "" {
				return
			}

			require.Contains(t, deploymentStates, definition.fallback, "wait state %q falls back to the unknown state %q", state, definition.fallback)
			require.Equal(t, deploymentStateKindAction, deploymentStates[definition.fallback].kind, "wait state %q falls back to %q, which is not an action", state, definition.fallback)
		})
	}
}

func Test_deploymentStatesAreAllReachable(t *testing.T) {
	reached := map[api.ServerDeploymentState]struct{}{}

	var walk func(state api.ServerDeploymentState)

	walk = func(state api.ServerDeploymentState) {
		_, ok := reached[state]
		if ok {
			return
		}

		reached[state] = struct{}{}

		definition := deploymentStates[state]
		if definition.next != "" {
			walk(definition.next)
		}

		if definition.fallback != "" {
			walk(definition.fallback)
		}
	}

	walk(api.ServerDeploymentStateRefreshBMCData)
	walk(api.ServerDeploymentStateCancel)
	walk(api.ServerDeploymentStateFailed)

	for state := range deploymentStates {
		require.Contains(t, reached, state, "state %q can not be reached from the entry states", state)
	}
}

// Test_deploymentStatesAreAllDispatched asserts, that every state of the state
// machine is handled by the dispatcher it is routed to by its kind. Every
// collaborator fails, so the outcome of the step is irrelevant, only whether the
// dispatcher recognized the state at all.
func Test_deploymentStatesAreAllDispatched(t *testing.T) {
	config.InitTest(t, &envMock.EnvironmentMock{
		IsIncusOSFunc: func() bool { return false },
	}, nil)

	server := provisioning.Server{
		Name:   "one",
		Status: api.ServerStatusDeploying,
		BMCConfig: api.BMCConfig{
			APIType:  api.BMCAPITypeRedfishV1Generic,
			Endpoint: "https://bmc.local:8443",
			Username: "admin",
			Password: "secret",
		},
	}

	repo := &repoMock.ServerRepoMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
			return nil, boom.Error
		},
		UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
			return boom.Error
		},
	}

	bmcClient := &adapterMock.BMCServerClientPortMock{
		GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
			return api.BMCData{}, boom.Error
		},
		BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
			return nil, boom.Error
		},
		ApplyBIOSAttributesFunc: func(ctx context.Context, server provisioning.Server, attributes map[string]any) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		},
		ApplySecureBootCertificatesFunc: func(ctx context.Context, server provisioning.Server, secureBoot api.BIOSSecureBoot) (bool, error) {
			return false, boom.Error
		},
		ServerPowerOnFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		},
		ServerPowerOffFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		},
		AttachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		},
		DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		},
		TaskStateFunc: func(ctx context.Context, server provisioning.Server, taskMonitor *provisioning.BMCTaskMonitor) (api.BMCTaskState, error) {
			return api.BMCTaskStateUnknown, boom.Error
		},
	}

	tokenSvc := &svcMock.TokenServiceMock{
		GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
			return nil, boom.Error
		},
		GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
			return nil, boom.Error
		},
		ResolveTokenSeedImageIDFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
			return "", boom.Error
		},
	}

	serverSvc := New(repo, nil, nil, tokenSvc, nil, nil, nil, tls.Certificate{},
		WithNow(func() time.Time { return deploymentTestNow }),
		AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
	)

	for state, definition := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			server := server
			server.StatusInternal = provisioning.ServerStatusInternal{
				Deployment: &provisioning.ServerDeployment{
					State:          state,
					StateEnteredAt: deploymentTestNow,
				},
			}

			var err error

			switch definition.kind {
			case deploymentStateKindAction:
				_, err = serverSvc.runDeploymentAction(t.Context(), slog.Default(), server)

			case deploymentStateKindWait:
				_, _, err = serverSvc.checkDeploymentWait(t.Context(), slog.Default(), server)

			case deploymentStateKindTerminal:
				return
			}

			if state == api.ServerDeploymentStateWaitRegistration {
				require.NoError(t, err, "the registration wait is answered from the server record alone")
				return
			}

			require.Error(t, err, "state %q reached none of the failing collaborators", state)
			require.NotContains(t, err.Error(), "is not an action", "action state %q is not dispatched", state)
			require.NotContains(t, err.Error(), "is not a wait", "wait state %q is not dispatched", state)
		})
	}
}
