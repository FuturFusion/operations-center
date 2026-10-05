package server

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"
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

func Test_deploymentStatesBIOSPass(t *testing.T) {
	tests := []struct {
		name  string
		state api.ServerDeploymentState

		want deploymentBIOSPass
	}{
		{name: "first pass power off", state: api.ServerDeploymentStatePowerOffBIOS, want: deploymentBIOSPassFirst},
		{name: "first pass wait power off", state: api.ServerDeploymentStateWaitPowerOffBIOS, want: deploymentBIOSPassFirst},
		{name: "first pass apply", state: api.ServerDeploymentStateApplyBIOS, want: deploymentBIOSPassFirst},
		{name: "first pass power on", state: api.ServerDeploymentStatePowerOnBIOS, want: deploymentBIOSPassFirst},
		{name: "first pass wait applied", state: api.ServerDeploymentStateWaitBIOSApplied, want: deploymentBIOSPassFirst},
		{name: "first pass verify", state: api.ServerDeploymentStateVerifyBIOS, want: deploymentBIOSPassFirst},
		{name: "deferred pass power off", state: api.ServerDeploymentStatePowerOffBIOSDeferred, want: deploymentBIOSPassDeferred},
		{name: "deferred pass wait power off", state: api.ServerDeploymentStateWaitPowerOffBIOSDeferred, want: deploymentBIOSPassDeferred},
		{name: "deferred pass apply", state: api.ServerDeploymentStateApplyBIOSDeferred, want: deploymentBIOSPassDeferred},
		{name: "deferred pass power on", state: api.ServerDeploymentStatePowerOnBIOSDeferred, want: deploymentBIOSPassDeferred},
		{name: "deferred pass wait applied", state: api.ServerDeploymentStateWaitBIOSAppliedDeferred, want: deploymentBIOSPassDeferred},
		{name: "deferred pass verify", state: api.ServerDeploymentStateVerifyBIOSDeferred, want: deploymentBIOSPassDeferred},
		{name: "unrelated state", state: api.ServerDeploymentStateAttachMedia, want: deploymentBIOSPassNone},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deploymentStates[tc.state].biosPass)
		})
	}
}

// deploymentTestRanks numbers the states along the happy path and the branches
// off it, so a skip can be held against the order the machine runs in. A state
// ranks behind every state, that leads to it.
func deploymentTestRanks(t *testing.T) map[api.ServerDeploymentState]int {
	t.Helper()

	ranks := map[api.ServerDeploymentState]int{}

	var visit func(state api.ServerDeploymentState, path []api.ServerDeploymentState)

	visit = func(state api.ServerDeploymentState, path []api.ServerDeploymentState) {
		require.NotContains(t, path, state, "the happy path revisits state %q", state)

		rank, seen := ranks[state]
		if seen && rank >= len(path) {
			return
		}

		ranks[state] = len(path)

		definition := deploymentStates[state]
		path = append(slices.Clone(path), state)

		for _, successor := range append([]api.ServerDeploymentState{definition.next}, definition.branches...) {
			if successor == "" {
				continue
			}

			visit(successor, path)
		}
	}

	visit(api.ServerDeploymentStateRefreshBMCData, nil)

	return ranks
}

// Test_deploymentStatesSkipForward asserts, that a state, which is passed by,
// names a state further along the happy path. That is what makes the loop in
// deploymentNextState settle, whatever the deployment looks like.
func Test_deploymentStatesSkipForward(t *testing.T) {
	ranks := deploymentTestRanks(t)

	for state, definition := range deploymentStates {
		if definition.enterState == nil {
			continue
		}

		t.Run(state.String(), func(t *testing.T) {
			require.Contains(t, ranks, state, "state %q is passed by, but is not on the happy path", state)

			for _, deployment := range deploymentDiagramSkipFlags() {
				entered := definition.enterState(deployment)
				if entered == state {
					continue
				}

				require.Contains(t, deploymentStates, entered, "state %q skips to the unknown state %q", state, entered)
				require.Greater(t, ranks[entered], ranks[state], "state %q skips to %q, which does not move forward", state, entered)
			}
		})
	}
}

// Test_deploymentNextStateSettles asserts, that the chain of skips always comes
// to rest, from every state and for every deployment.
func Test_deploymentNextStateSettles(t *testing.T) {
	for state := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			for _, deployment := range deploymentDiagramSkipFlags() {
				next := deploymentNextState(deployment, state)

				require.Contains(t, deploymentStates, next, "state %q settles on the unknown state %q", state, next)
				require.Equal(t, next, deploymentNextState(deployment, next), "state %q settles on %q, which is passed by itself", state, next)
			}
		})
	}
}

func deploymentTestLeafFields(t *testing.T, value reflect.Value, path string, visit func(path string, field reflect.Value)) {
	t.Helper()

	if value.Kind() != reflect.Struct || value.Type() == reflect.TypeFor[time.Time]() {
		visit(path, value)

		return
	}

	for i := range value.NumField() {
		deploymentTestLeafFields(t, value.Field(i), path+"."+value.Type().Field(i).Name, visit)
	}
}

func deploymentTestSetNonZero(t *testing.T, path string, field reflect.Value) {
	t.Helper()

	if field.Type() == reflect.TypeFor[time.Time]() {
		field.Set(reflect.ValueOf(deploymentTestNow))

		return
	}

	switch field.Kind() {
	case reflect.Bool:
		field.SetBool(true)

	case reflect.String:
		field.SetString("set")

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		field.SetInt(1)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		field.SetUint(1)

	case reflect.Float32, reflect.Float64:
		field.SetFloat(1)

	case reflect.Array:
		deploymentTestSetNonZero(t, path, field.Index(0))

	case reflect.Slice:
		field.Set(reflect.MakeSlice(field.Type(), 1, 1))

	case reflect.Map:
		field.Set(reflect.MakeMapWithSize(field.Type(), 1))
		field.SetMapIndex(reflect.Zero(field.Type().Key()), reflect.Zero(field.Type().Elem()))

	case reflect.Pointer:
		field.Set(reflect.New(field.Type().Elem()))

	default:
		require.FailNow(t, "unsupported field kind", "field %q is of kind %s, which the test does not know how to set", path, field.Kind())
	}
}

func Test_deploymentDiagramSkipFlagsCoverWhatEnterStateReads(t *testing.T) {
	combinations := deploymentDiagramSkipFlags()

	enumerated := map[string]bool{}
	allSet := reflect.ValueOf(combinations[len(combinations)-1]).Elem()

	deploymentTestLeafFields(t, reflect.ValueOf(combinations[0]).Elem(), "deployment", func(path string, field reflect.Value) {
		enumerated[path] = false
	})

	deploymentTestLeafFields(t, allSet, "deployment", func(path string, field reflect.Value) {
		enumerated[path] = !field.IsZero()
	})

	for state, definition := range deploymentStates {
		if definition.enterState == nil {
			continue
		}

		t.Run(state.String(), func(t *testing.T) {
			for _, base := range combinations {
				want := definition.enterState(base)

				changed := *base
				deploymentTestLeafFields(t, reflect.ValueOf(&changed).Elem(), "deployment", func(path string, field reflect.Value) {
					if enumerated[path] {
						return
					}

					before := reflect.New(field.Type()).Elem()
					before.Set(field)

					deploymentTestSetNonZero(t, path, field)

					require.Equal(t, want, definition.enterState(&changed), "state %q is entered or passed by depending on %q, which deploymentDiagramSkipFlags does not enumerate", state, path)

					field.Set(before)
				})
			}
		})
	}
}

func Test_deploymentStatesEnterStateOutcomes(t *testing.T) {
	for state, definition := range deploymentStates {
		if definition.enterState == nil {
			require.Empty(t, definition.branches, "state %q declares branches, but has nothing routing to them", state)

			continue
		}

		t.Run(state.String(), func(t *testing.T) {
			outcomes := map[api.ServerDeploymentState]struct{}{}
			for _, deployment := range deploymentDiagramSkipFlags() {
				outcomes[definition.enterState(deployment)] = struct{}{}
			}

			require.Contains(t, outcomes, state, "state %q is never entered", state)

			for _, branch := range definition.branches {
				require.Contains(t, outcomes, branch, "state %q declares the branch %q, but never routes to it", state, branch)
			}

			delete(outcomes, state)

			for _, branch := range definition.branches {
				delete(outcomes, branch)
			}

			require.LessOrEqual(t, len(outcomes), 1, "state %q routes to %v, which are neither declared as branches nor a single state to skip to", state, slices.Collect(maps.Keys(outcomes)))
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

func Test_deploymentInstallOSObserved(t *testing.T) {
	anchored := deploymentTestNow

	tests := []struct {
		name     string
		snapshot provisioning.ServerDeploymentBMCSnapshot
		current  api.BMCData

		want bool
	}{
		{
			name:     "the BMC reports no boot progress",
			snapshot: provisioning.ServerDeploymentBMCSnapshot{Taken: anchored},
			current:  api.BMCData{},

			want: false,
		},
		{
			name:     "the firmware is still running the power on self test",
			snapshot: provisioning.ServerDeploymentBMCSnapshot{Taken: anchored},
			current:  api.BMCData{ServerBootProgress: api.BMCBootProgress{LastState: "MemoryInitializationStarted", LastStateTime: anchored.Add(time.Minute)}},

			want: false,
		},
		{
			name:     "the installer is running",
			snapshot: provisioning.ServerDeploymentBMCSnapshot{Taken: anchored},
			current:  api.BMCData{ServerBootProgress: api.BMCBootProgress{LastState: "OSRunning", LastStateTime: anchored.Add(time.Minute)}},

			want: true,
		},
		{
			name:     "a BMC, that kept the state of the boot before the install wait",
			snapshot: provisioning.ServerDeploymentBMCSnapshot{Taken: anchored},
			current:  api.BMCData{ServerBootProgress: api.BMCBootProgress{LastState: "OSRunning", LastStateTime: anchored.Add(-time.Hour)}},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deployment := &provisioning.ServerDeployment{InstallSnapshot: tc.snapshot}

			require.Equal(t, tc.want, deploymentInstallOSObserved(deployment, tc.current), "only a boot, that ran the installer, may let a reboot end the install wait")
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
		{name: "verification of the first pass", state: api.ServerDeploymentStateVerifyBIOS, want: deployment.BIOSAttributes},
		{name: "deferred pass", state: api.ServerDeploymentStateApplyBIOSDeferred, want: deployment.BIOSDeferredAttributes},
		{name: "verification of the deferred pass", state: api.ServerDeploymentStateVerifyBIOSDeferred, want: deployment.BIOSDeferredAttributes},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deploymentBIOSAttributes(deploymentStates[tc.state], &deployment)

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
			got := deploymentMediaBytesRequired(tc.size, config.ServerDeploymentMediaMinBytesRead)

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
			got := deploymentMediaReadOut(tc.progress, config.ServerDeploymentMediaMinBytesRead)

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
			got := deploymentMediaIdle(deploymentTestNow, tc.progress, config.ServerDeploymentMediaIdlePeriod)

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
			got := deploymentInstallCouldBeDone(deploymentTestNow, &provisioning.ServerDeployment{StateEnteredAt: tc.stateEnteredAt}, config.ServerDeploymentMinInstallDuration)

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

			rebooted, observed := deploymentRebootObserved(deploymentTestNow, &deployment, tc.current, config.ServerDeploymentRebootObservationWindow)

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

			mutate := deploymentSettleSnapshot(deploymentTestNow, &deployment, data, config.ServerDeploymentSettleDelay, func(deployment *provisioning.ServerDeployment, snapshot provisioning.ServerDeploymentBMCSnapshot) {
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

func Test_checkDeploymentPoweredOff(t *testing.T) {
	config.InitTest(t, &envMock.EnvironmentMock{
		IsIncusOSFunc: func() bool { return false },
	}, nil)

	tests := []struct {
		name            string
		powerState      string
		poweredOffSince time.Time

		wantOutcome         deploymentWaitOutcome
		wantPoweredOffSince time.Time
	}{
		{
			name:       "the server is still powered on",
			powerState: bmcPowerStateOn,

			wantOutcome: deploymentWaitPending,
		},
		{
			name:            "the server is powered on again after it had been reported powered off",
			powerState:      bmcPowerStateOn,
			poweredOffSince: deploymentTestNow.Add(-config.ServerDeploymentPowerOffSettleDelay),

			wantOutcome:         deploymentWaitRevert,
			wantPoweredOffSince: deploymentTestNow.Add(-config.ServerDeploymentPowerOffSettleDelay),
		},
		{
			name:       "the server is reported powered off for the first time",
			powerState: bmcPowerStateOff,

			wantOutcome:         deploymentWaitPending,
			wantPoweredOffSince: deploymentTestNow,
		},
		{
			name:            "the power off has not settled yet",
			powerState:      bmcPowerStateOff,
			poweredOffSince: deploymentTestNow.Add(-config.ServerDeploymentPowerOffSettleDelay + time.Second),

			wantOutcome:         deploymentWaitPending,
			wantPoweredOffSince: deploymentTestNow.Add(-config.ServerDeploymentPowerOffSettleDelay + time.Second),
		},
		{
			name:            "the power off has settled",
			powerState:      bmcPowerStateOff,
			poweredOffSince: deploymentTestNow.Add(-config.ServerDeploymentPowerOffSettleDelay),

			wantOutcome:         deploymentWaitMet,
			wantPoweredOffSince: deploymentTestNow.Add(-config.ServerDeploymentPowerOffSettleDelay),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusDeploying,
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local:8443",
					Username: "admin",
					Password: "secret",
				},
				// The BMC data is fresh, so the wait works off the record
				// instead of asking the BMC again.
				BMCData: api.BMCData{ServerPowerState: tc.powerState, LastUpdated: deploymentTestNow},
				StatusInternal: provisioning.ServerStatusInternal{
					Deployment: &provisioning.ServerDeployment{
						State:           api.ServerDeploymentStateWaitPowerOffSecureBootReset,
						StateEnteredAt:  deploymentTestNow,
						PoweredOffSince: tc.poweredOffSince,
					},
				},
			}

			// The wait only observes, a power off by the wait would call into
			// the BMC client, which has no function set up for it.
			bmcClient := &adapterMock.BMCServerClientPortMock{}

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &server, nil
				},
			}

			serverSvc := New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				WithNow(func() time.Time { return deploymentTestNow }),
				AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			outcome, mutate, err := serverSvc.checkDeploymentPoweredOff(t.Context(), slog.Default(), server, deploymentStates[api.ServerDeploymentStateWaitPowerOffSecureBootReset])
			require.NoError(t, err)
			require.Equal(t, tc.wantOutcome, outcome)

			deployment := *server.StatusInternal.Deployment
			if mutate != nil {
				mutate(&deployment)
			}

			require.Equal(t, tc.wantPoweredOffSince, deployment.PoweredOffSince)
		})
	}
}

func Test_deploymentBMCConditions(t *testing.T) {
	deployment := provisioning.ServerDeployment{
		Request:  provisioning.ServerDeploymentRequest{VirtualMediaID: "system:1"},
		MediaURL: "https://oc.example.com:8443/one.iso",
	}

	tests := []struct {
		name      string
		condition deploymentBMCCondition
		data      api.BMCData

		want bool
	}{
		{
			name:      "power is off",
			condition: deploymentPowerIsOff,
			data:      api.BMCData{ServerPowerState: bmcPowerStateOff},

			want: true,
		},
		{
			name:      "power is still on",
			condition: deploymentCancelSettled,
			data:      api.BMCData{ServerPowerState: bmcPowerStateOn},

			want: false,
		},
		{
			name:      "the cancellation has powered the server off and ejected the media",
			condition: deploymentCancelSettled,
			data: api.BMCData{
				ServerPowerState: bmcPowerStateOff,
				VirtualMedia: map[string]api.BMCVirtualMedia{
					"system:1": {ID: "system:1"},
				},
			},

			want: true,
		},
		{
			name:      "the cancellation has powered the server off, but the media is still inserted",
			condition: deploymentCancelSettled,
			data: api.BMCData{
				ServerPowerState: bmcPowerStateOff,
				VirtualMedia: map[string]api.BMCVirtualMedia{
					"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/one.iso"},
				},
			},

			want: false,
		},
		{
			name:      "no media is inserted",
			condition: deploymentNoMediaInserted,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1"},
			}},

			want: true,
		},
		{
			name:      "another media is still inserted",
			condition: deploymentNoMediaInserted,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", Inserted: true},
			}},

			want: false,
		},
		{
			name:      "the media is ejected",
			condition: deploymentMediaEjected,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1"},
			}},

			want: true,
		},
		{
			name:      "the media device is gone",
			condition: deploymentMediaEjected,
			data:      api.BMCData{},

			want: true,
		},
		{
			name:      "the media is still inserted",
			condition: deploymentMediaEjected,
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/one.iso"},
			}},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.condition.met(&deployment, tc.data))
		})
	}
}

func Test_deploymentBMCConditionsDeclareTheirParts(t *testing.T) {
	conditions := map[string]deploymentBMCCondition{
		"power is off":                  deploymentPowerIsOff,
		"cancel settled":                deploymentCancelSettled,
		"no media inserted":             deploymentNoMediaInserted,
		"media holds image":             deploymentMediaHoldsImage,
		"media ejected":                 deploymentMediaEjected,
		"secure boot media holds image": isDeploymentSecureBootMediaHoldingImage,
		"in secure boot setup mode":     isDeploymentInSecureBootSetupMode,
	}

	for name, condition := range conditions {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, condition.met)
			require.NotEmpty(t, condition.requires)

			for _, part := range condition.requires {
				require.Contains(t, api.BMCDataParts, part)
			}
		})
	}
}

func Test_deploymentMediaHoldsImage(t *testing.T) {
	deployment := provisioning.ServerDeployment{
		Request:  provisioning.ServerDeploymentRequest{VirtualMediaID: "system:1"},
		MediaURL: "https://oc.example.com:8443/one.iso",
	}

	tests := []struct {
		name string
		data api.BMCData

		want bool
	}{
		{
			name: "the media holds the image",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/one.iso"},
			}},

			want: true,
		},
		{
			name: "the media holds another image",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", Inserted: true, Image: "https://oc.example.com:8443/other.iso"},
			}},

			want: false,
		},
		{
			name: "nothing is inserted",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1"},
			}},

			want: false,
		},
		{
			name: "the media device is gone",
			data: api.BMCData{},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deploymentMediaHoldsImage.met(&deployment, tc.data))
		})
	}
}

func Test_selectVirtualMediaID(t *testing.T) {
	tests := []struct {
		name             string
		data             api.BMCData
		imageType        api.ImageType
		requireStreaming bool

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "the only device takes the image",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
			}},
			imageType: api.ImageTypeRaw,

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "the only device does not take the image",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
			}},
			imageType: api.ImageTypeISO,

			assertErr: errassert.OperationNotPermittedErrorContains(`manager:1 (USBStick)`),
		},
		{
			name: "the only optical device",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"CD", "DVD"}},
			}},
			imageType: api.ImageTypeISO,

			want:      "system:1",
			assertErr: require.NoError,
		},
		{
			name: "the same devices, but a raw image needs the USB one",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"CD", "DVD"}},
			}},
			imageType: api.ImageTypeRaw,

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name:      "no device at all",
			data:      api.BMCData{},
			imageType: api.ImageTypeISO,

			assertErr: errassert.NotFoundError,
		},
		{
			name: "several optical devices",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"system:1": {ID: "system:1", MediaTypes: []string{"CD"}},
				"system:2": {ID: "system:2", MediaTypes: []string{"DVD"}},
			}},
			imageType: api.ImageTypeISO,

			want:      "system:1",
			assertErr: require.NoError,
		},
		{
			name: "the device saying nothing wins over the one saying it does not take the image",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"manager:2": {ID: "manager:2"},
			}},
			imageType: api.ImageTypeISO,

			want:      "manager:2",
			assertErr: require.NoError,
		},
		{
			name: "the optical device of the system wins over the one of the manager",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"CD", "DVD"}},
				"system:2":  {ID: "system:2", MediaTypes: []string{"CD", "DVD"}},
			}},
			imageType: api.ImageTypeISO,

			want:      "system:2",
			assertErr: require.NoError,
		},
		{
			name: "the optical device of the manager wins over the non optical one of the system",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"CD", "DVD"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"USBStick"}},
			}},
			imageType: api.ImageTypeISO,

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "no device advertises anything, the one of the system wins",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1"},
				"system:1":  {ID: "system:1"},
			}},
			imageType: api.ImageTypeISO,

			want:      "system:1",
			assertErr: require.NoError,
		},
		{
			name: "a device advertising the media type wins over one advertising nothing",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"system:1":  {ID: "system:1"},
			}},
			imageType: api.ImageTypeRaw,

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "a streaming device wins over an uploading one, when the read progress is needed",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"CD", "DVD"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"CD", "DVD"}, TransferMethod: "Upload"},
			}},
			imageType:        api.ImageTypeISO,
			requireStreaming: true,

			want:      "manager:1",
			assertErr: require.NoError,
		},
		{
			name: "the uploading device of the system is preferred, when the read progress is not needed",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"CD", "DVD"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"CD", "DVD"}, TransferMethod: "Upload"},
			}},
			imageType: api.ImageTypeISO,

			want:      "system:1",
			assertErr: require.NoError,
		},
		{
			name: "an uploading device is picked, when it is the only one taking the image",
			data: api.BMCData{VirtualMedia: map[string]api.BMCVirtualMedia{
				"manager:1": {ID: "manager:1", MediaTypes: []string{"USBStick"}},
				"system:1":  {ID: "system:1", MediaTypes: []string{"CD", "DVD"}, TransferMethod: "Upload"},
			}},
			imageType:        api.ImageTypeISO,
			requireStreaming: true,

			want:      "system:1",
			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectVirtualMediaID(tc.data, tc.imageType, tc.requireStreaming)

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

func Test_deploymentFatalError(t *testing.T) {
	err := error(deploymentFatalError{err: boom.Error})

	require.Equal(t, boom.Error.Error(), err.Error(), "the sentinel reports the message of the error it carries")
	require.ErrorIs(t, err, boom.Error, "the wrapped error stays inspectable")
	require.False(t, domain.IsRetryableError(err), "a fatal error is never retried")

	_, ok := errors.AsType[deploymentFatalError](fmt.Errorf("wrapped: %w", err))
	require.True(t, ok, "the sentinel is found through another wrapping")

	_, ok = errors.AsType[deploymentFatalError](boom.Error)
	require.False(t, ok, "a plain error does not end the deployment")
}

// Test_deploymentStates asserts the invariants of the deployment state machine,
// which the dispatcher relies on but can not check itself.
func Test_deploymentStates(t *testing.T) {
	for state, definition := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			var unmarshalled api.ServerDeploymentState
			require.NoError(t, unmarshalled.UnmarshalText([]byte(state)), "state %q is not a known deployment state", state)

			if definition.kind != deploymentStateKindAction {
				require.Nil(t, definition.prepare, "state %q is not an action, but prepares one", state)
			}

			if definition.kind == deploymentStateKindTerminal {
				require.True(t, state.IsTerminal(), "terminal state %q does not report itself as terminal", state)
				require.Empty(t, definition.next, "terminal state %q leads somewhere", state)
				require.Empty(t, definition.fallback, "terminal state %q has a fallback", state)
				require.Zero(t, definition.timeout, "terminal state %q has a timeout", state)
				require.Nil(t, definition.action, "terminal state %q has an action", state)
				require.Nil(t, definition.wait, "terminal state %q has a wait", state)
				require.Empty(t, definition.revert, "terminal state %q reverts", state)
				require.False(t, definition.powerOff, "terminal state %q powers off", state)
				require.Zero(t, definition.retries, "terminal state %q has a retry budget", state)
				require.NotEmpty(t, definition.status, "terminal state %q reports no server status", state)

				return
			}

			require.False(t, state.IsTerminal(), "non terminal state %q reports itself as terminal", state)
			require.NotEmpty(t, definition.detail, "state %q reports no status detail", state)
			require.Empty(t, definition.status, "non terminal state %q reports a server status, which would take the server out of deploying", state)
			require.NotEqual(t, state, definition.next, "state %q leads to itself", state)
			require.Contains(t, deploymentStates, definition.next, "state %q leads to the unknown state %q", state, definition.next)

			for _, branch := range definition.branches {
				require.NotEqual(t, state, branch, "state %q branches to itself", state)
				require.Contains(t, deploymentStates, branch, "state %q branches to the unknown state %q", state, branch)
			}

			if definition.retryFrom != "" {
				require.Contains(t, deploymentStates, definition.retryFrom, "state %q routes back to the unknown state %q", state, definition.retryFrom)
				require.Equal(t, deploymentStateKindAction, deploymentStates[definition.retryFrom].kind, "state %q routes back to %q, which is not an action", state, definition.retryFrom)
			}

			if definition.kind == deploymentStateKindAction {
				require.NotNil(t, definition.action, "action state %q performs nothing", state)
				require.Empty(t, definition.fallback, "action state %q has a fallback", state)
				require.Zero(t, definition.timeout, "action state %q has a timeout", state)
				require.Positive(t, definition.retries, "action state %q has no retry budget", state)
				require.Zero(t, definition.settleDelay, "action state %q has a settle delay", state)
				require.Zero(t, definition.powerOffSettleDelay, "action state %q has a power off settle delay", state)
				require.Zero(t, definition.rebootWindow, "action state %q has a reboot window", state)
				require.Zero(t, definition.install, "action state %q has install thresholds", state)
				require.Nil(t, definition.wait, "action state %q has a wait", state)
				require.Empty(t, definition.revert, "action state %q reverts", state)

				return
			}

			require.NotNil(t, definition.wait, "wait state %q waits for nothing", state)
			require.Nil(t, definition.action, "wait state %q has an action", state)
			require.Positive(t, definition.timeout, "wait state %q is not bounded by a timeout", state)
			require.False(t, definition.powerOff, "wait state %q powers off", state)

			if definition.revert != "" {
				require.Contains(t, deploymentStates, definition.revert, "wait state %q reverts to the unknown state %q", state, definition.revert)
				require.Equal(t, deploymentStateKindAction, deploymentStates[definition.revert].kind, "wait state %q reverts to %q, which is not an action", state, definition.revert)
				require.Positive(t, definition.retries, "wait state %q reverts, but has no retry budget", state)
				require.NotEmpty(t, definition.revertReason, "wait state %q reverts, but does not say why", state)
				require.True(t, deploymentTestLeadsTo(definition.revert, state), "wait state %q reverts to %q, which does not lead back to it", state, definition.revert)
			}

			if definition.fallback == "" {
				if definition.revert == "" {
					require.Zero(t, definition.retries, "wait state %q has no trigger to fall back to or revert to, so its retry budget can not be spent", state)
				}

				return
			}

			require.Positive(t, definition.retries, "wait state %q falls back to a trigger, but has no retry budget", state)

			require.Contains(t, deploymentStates, definition.fallback, "wait state %q falls back to the unknown state %q", state, definition.fallback)
			require.Equal(t, deploymentStateKindAction, deploymentStates[definition.fallback].kind, "wait state %q falls back to %q, which is not an action", state, definition.fallback)
			require.True(t, deploymentTestLeadsTo(definition.fallback, state), "wait state %q falls back to %q, which does not lead back to it", state, definition.fallback)
		})
	}
}

func deploymentTestLeadsTo(from api.ServerDeploymentState, to api.ServerDeploymentState) bool {
	for range len(deploymentStates) {
		if from == to {
			return true
		}

		from = deploymentStates[from].next
	}

	return false
}

// Test_deploymentStatesSecureBootRecordsItsAttempt asserts, that the enrollment
// of the secure boot certificates records, that it is about to run. The record
// is what tells a re-issued enrollment, that an earlier attempt may have written
// the key databases already, which the BMC does not report anymore.
func Test_deploymentStatesSecureBootRecordsItsAttempt(t *testing.T) {
	prepare := deploymentStates[api.ServerDeploymentStateSecureBoot].prepare
	require.NotNil(t, prepare, "the enrollment does not record its attempt")

	var deployment provisioning.ServerDeployment

	prepare(&deployment)

	require.True(t, deployment.SecureBootAttempted)
}

// Test_deploymentStatesCancelPhase asserts, that exactly the states, which clean
// a cancelled deployment up, are exempt from being preempted by a cancellation.
// A state marked wrongly either never cancels or never finishes cancelling.
func Test_deploymentStatesCancelPhase(t *testing.T) {
	want := []api.ServerDeploymentState{
		api.ServerDeploymentStateCancel,
		api.ServerDeploymentStateWaitCancel,
	}

	for state, definition := range deploymentStates {
		require.Equal(t, slices.Contains(want, state), definition.cancelPhase, "state %q is marked as a cancel phase wrongly", state)
	}
}

// Test_deploymentStatesTuningIsDeclaredWhereItIsRead asserts, that a threshold is
// declared exactly at the states, whose wait reads it. A threshold left at zero
// would silently turn the signal it guards into an immediate accept.
func Test_deploymentStatesTuningIsDeclaredWhereItIsRead(t *testing.T) {
	wantSettleDelay := []api.ServerDeploymentState{
		api.ServerDeploymentStateWaitBIOSApplied,
		api.ServerDeploymentStateWaitBIOSAppliedDeferred,
		api.ServerDeploymentStateWaitSecureBootReset,
		api.ServerDeploymentStateWaitSecureBootEnrolled,
		api.ServerDeploymentStateWaitSecureBootSettled,
		api.ServerDeploymentStateWaitInstall,
		api.ServerDeploymentStateWaitReboot,
	}

	wantRebootWindow := []api.ServerDeploymentState{
		api.ServerDeploymentStateWaitSecureBootSettled,
		api.ServerDeploymentStateWaitReboot,
	}

	wantPowerOffSettleDelay := []api.ServerDeploymentState{
		api.ServerDeploymentStateWaitPowerOffBIOS,
		api.ServerDeploymentStateWaitPowerOffBIOSDeferred,
		api.ServerDeploymentStateWaitPowerOffSecureBoot,
		api.ServerDeploymentStateWaitPowerOffSecureBootReset,
		api.ServerDeploymentStateWaitPowerOffSecureBootMedia,
		api.ServerDeploymentStateWaitPowerOffSecureBootSettled,
	}

	wantInstall := []api.ServerDeploymentState{
		api.ServerDeploymentStateWaitInstall,
	}

	for state, definition := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			if slices.Contains(wantSettleDelay, state) {
				require.Positive(t, definition.settleDelay, "state %q reads a settle delay, but declares none", state)
			} else {
				require.Zero(t, definition.settleDelay, "state %q declares a settle delay, which nothing reads", state)
			}

			if slices.Contains(wantPowerOffSettleDelay, state) {
				require.Positive(t, definition.powerOffSettleDelay, "state %q reads a power off settle delay, but declares none", state)
			} else {
				require.Zero(t, definition.powerOffSettleDelay, "state %q declares a power off settle delay, which nothing reads", state)
			}

			if slices.Contains(wantRebootWindow, state) {
				require.Positive(t, definition.rebootWindow, "state %q reads a reboot window, but declares none", state)
			} else {
				require.Zero(t, definition.rebootWindow, "state %q declares a reboot window, which nothing reads", state)
			}

			if slices.Contains(wantInstall, state) {
				require.Positive(t, definition.install.minDuration, "state %q declares no minimum install duration", state)
				require.Positive(t, definition.install.rebootFallbackDelay, "state %q declares no reboot fallback delay", state)
				require.Positive(t, definition.install.mediaIdlePeriod, "state %q declares no media idle period", state)
				require.Positive(t, definition.install.mediaMinBytesRead, "state %q declares no minimum of media read", state)
			} else {
				require.Zero(t, definition.install, "state %q declares install thresholds, which nothing reads", state)
			}
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

		for _, branch := range definition.branches {
			walk(branch)
		}

		if definition.revert != "" {
			walk(definition.revert)
		}
	}

	walk(api.ServerDeploymentStateRefreshBMCData)
	walk(api.ServerDeploymentStateCancel)
	walk(api.ServerDeploymentStateFailed)

	for state, definition := range deploymentStates {
		require.Contains(t, reached, state, "state %q can not be reached from the entry states", state)

		if definition.retryFrom != "" {
			require.Contains(t, reached, definition.retryFrom, "state %q routes back to the unreachable state %q", state, definition.retryFrom)
		}

		if definition.enterState == nil {
			continue
		}

		for _, deployment := range deploymentDiagramSkipFlags() {
			require.Contains(t, reached, definition.enterState(deployment), "state %q skips to an unreachable state", state)
		}
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
		EnableSecureBootFunc: func(ctx context.Context, server provisioning.Server) (bool, error) {
			return false, boom.Error
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
				require.NotNil(t, definition.action, "action state %q declares no action", state)

				_, err = definition.action(serverSvc, t.Context(), slog.Default(), server, definition)

			case deploymentStateKindWait:
				require.NotNil(t, definition.wait, "wait state %q declares no wait", state)

				_, _, err = definition.wait(serverSvc, t.Context(), slog.Default(), server, definition)

			case deploymentStateKindTerminal:
				return
			}

			if state == api.ServerDeploymentStateWaitRegistration {
				require.NoError(t, err, "the registration wait is answered from the server record alone")
				return
			}

			require.Error(t, err, "state %q reached none of the failing collaborators", state)
		})
	}
}

func Test_deploymentBMCData_requiresParts(t *testing.T) {
	tests := []struct {
		name     string
		data     api.BMCData
		requires []api.BMCDataPart

		assertErr require.ErrorAssertionFunc
		wantErr   string
	}{
		{
			name:     "the part asked for has been collected",
			data:     api.BMCData{ServerPowerState: "Off"},
			requires: []api.BMCDataPart{api.BMCDataPartSystem},

			assertErr: require.NoError,
		},
		{
			name: "the part asked for could not be collected",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503: IDRAC.2.8.SYS518",
			}},
			requires: []api.BMCDataPart{api.BMCDataPartVirtualMedia},

			assertErr: require.Error,
			wantErr:   `The BMC of server "one" did not report virtual_media (BMC returned HTTP 503: IDRAC.2.8.SYS518)`,
		},
		{
			name: "another part could not be collected",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartBIOSAttributes: "BMC returned HTTP 503",
			}},
			requires: []api.BMCDataPart{api.BMCDataPartVirtualMedia},

			assertErr: require.NoError,
		},
		{
			name: "nothing is read off the BMC data",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
			}},
			requires: nil,

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &serverService{now: func() time.Time { return deploymentTestNow }}

			// The data is fresh, so it is taken as is and nothing is collected
			// from the BMC for this.
			tc.data.LastUpdated = deploymentTestNow

			server := provisioning.Server{Name: "one", BMCData: tc.data}

			current, err := s.deploymentBMCData(context.Background(), server, tc.requires)

			tc.assertErr(t, err)

			if tc.wantErr == "" {
				require.NotNil(t, current)

				return
			}

			require.Nil(t, current)
			require.Contains(t, err.Error(), tc.wantErr)
			require.True(t, domain.IsRetryableError(err), "a BMC, that could not be asked, is asked again")
		})
	}
}

// Test_deploymentStatesDiagramMetadata asserts, that every state carries what
// the generated state diagram needs, so a state can not be added to the table
// without showing up in the documentation.
func Test_deploymentStatesDiagramMetadata(t *testing.T) {
	for state, definition := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			require.NotEmpty(t, definition.label, "state %q has no label to draw it by", state)

			condition, err := deploymentDiagramCondition(definition)
			require.NoError(t, err, "the condition of state %q does not render", state)
			require.NotContains(t, condition, "<no value>", "the condition of state %q references something the view does not hold", state)

			if definition.enterState != nil {
				require.NotEmpty(t, definition.skipReason, "state %q can be passed by, but does not say why", state)
			}

			if len(definition.branches) > 0 {
				require.NotEmpty(t, definition.branchReason, "state %q branches off, but does not say why", state)
			}

			if definition.retryFrom != "" {
				require.NotEmpty(t, definition.retryReason, "state %q routes back to %q, but does not say why", state, definition.retryFrom)
			}

			if definition.kind == deploymentStateKindWait {
				require.NotEmpty(t, definition.condition, "wait state %q does not say, what satisfies it", state)
			}
		})
	}
}

func Test_deploymentDiagramDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration

		want string
	}{
		{name: "zero", duration: 0, want: "0s"},
		{name: "negative", duration: -time.Minute, want: "0s"},
		{name: "seconds", duration: 10 * time.Second, want: "10s"},
		{name: "whole minutes", duration: 10 * time.Minute, want: "10m"},
		{name: "whole hours", duration: 2 * time.Hour, want: "2h"},
		{name: "hours and minutes", duration: 90 * time.Minute, want: "1h30m"},
		{name: "every unit", duration: time.Hour + 2*time.Minute + 3*time.Second, want: "1h2m3s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deploymentDiagramDuration(tc.duration))
		})
	}
}

// Test_DeploymentStateDiagram asserts, that the rendered diagram covers the
// table and nothing but the table, and that it does not depend on the iteration
// order of the map.
func Test_DeploymentStateDiagram(t *testing.T) {
	diagram, err := DeploymentStateDiagram()
	require.NoError(t, err)

	again, err := DeploymentStateDiagram()
	require.NoError(t, err)
	require.Equal(t, diagram, again, "the diagram is not rendered deterministically")

	declared := map[string]struct{}{}

	for line := range strings.SplitSeq(diagram, "\n") {
		line = strings.TrimSpace(line)

		id, ok := strings.CutPrefix(line, "state ")
		if ok {
			_, id, _ = strings.Cut(id, " as ")
			declared[id] = struct{}{}

			continue
		}

		from, rest, ok := strings.Cut(line, " --> ")
		if !ok {
			continue
		}

		to, _, _ := strings.Cut(rest, ": ")

		for _, id := range []string{from, to} {
			if id == "[*]" {
				continue
			}

			require.Contains(t, declared, id, "the diagram draws an edge to %q, which it does not declare", id)
		}
	}

	require.Len(t, declared, len(deploymentStates), "the diagram does not declare every state of the table")

	for state := range deploymentStates {
		require.Contains(t, declared, deploymentDiagramID(state), "the diagram leaves state %q out", state)
	}
}

func Test_deploymentDiagramEdges(t *testing.T) {
	edges, err := deploymentDiagramEdges(deploymentDiagramOrder())
	require.NoError(t, err)

	drawn := map[api.ServerDeploymentState]map[string][]string{}
	for _, edge := range edges {
		from := api.ServerDeploymentState(edge.from)
		if drawn[from] == nil {
			drawn[from] = map[string][]string{}
		}

		drawn[from][edge.to] = append(drawn[from][edge.to], edge.label)
	}

	for state, definition := range deploymentStates {
		t.Run(state.String(), func(t *testing.T) {
			want := map[string]struct{}{}

			if definition.kind == deploymentStateKindTerminal {
				want["[*]"] = struct{}{}
			} else {
				for _, deployment := range deploymentDiagramSkipFlags() {
					want[string(deploymentNextState(deployment, definition.next))] = struct{}{}
				}
			}

			if definition.retryFrom != "" {
				want[string(definition.retryFrom)] = struct{}{}
				require.Contains(t, drawn[state][string(definition.retryFrom)], definition.retryReason, "the edge of state %q back to %q is not labeled with its reason", state, definition.retryFrom)
			}

			if definition.revert != "" {
				want[string(definition.revert)] = struct{}{}
				require.Contains(t, drawn[state][string(definition.revert)], definition.revertReason, "the edge of wait %q reverting to %q is not labeled with its reason", state, definition.revert)
			}

			if definition.kind == deploymentStateKindWait {
				timeout := cmp.Or(definition.fallback, api.ServerDeploymentStateFailed)
				want[string(timeout)] = struct{}{}
				require.Contains(t, drawn[state][string(timeout)], "timeout ("+deploymentDiagramDuration(definition.timeout)+")", "the diagram does not draw the timeout of wait %q to %q", state, timeout)
			}

			require.ElementsMatch(t, slices.Collect(maps.Keys(want)), slices.Collect(maps.Keys(drawn[state])), "the diagram does not draw exactly the transitions of state %q", state)
		})
	}
}

func Test_checkDeploymentRebooted(t *testing.T) {
	settleDelay := deploymentStates[api.ServerDeploymentStateWaitReboot].settleDelay

	tests := []struct {
		name          string
		powerState    string
		enteredAgo    time.Duration
		noSettleDelay bool
		staleBMCData  bool

		wantOutcome deploymentWaitOutcome
	}{
		{
			name:       "the server stayed off for the settle delay",
			powerState: bmcPowerStateOff,
			enteredAgo: settleDelay,

			wantOutcome: deploymentWaitRevert,
		},
		{
			name:       "the server is off, but the power on has not been given the settle delay yet",
			powerState: bmcPowerStateOff,
			enteredAgo: settleDelay - time.Second,

			wantOutcome: deploymentWaitPending,
		},
		{
			name:          "the server is reported off by BMC data collected before the state was entered",
			powerState:    bmcPowerStateOff,
			enteredAgo:    time.Second,
			noSettleDelay: true,
			staleBMCData:  true,

			wantOutcome: deploymentWaitPending,
		},
		{
			name:       "the server is up, but the reboot is not observed yet",
			powerState: bmcPowerStateOn,
			enteredAgo: time.Second,

			wantOutcome: deploymentWaitPending,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stateEnteredAt := deploymentTestNow.Add(-tc.enteredAgo)

			lastUpdated := deploymentTestNow
			if tc.staleBMCData {
				lastUpdated = stateEnteredAt
			}

			server := provisioning.Server{
				Name:    "one",
				Status:  api.ServerStatusDeploying,
				BMCData: api.BMCData{ServerPowerState: tc.powerState, ServerLastResetTime: stateEnteredAt, LastUpdated: lastUpdated},
				StatusInternal: provisioning.ServerStatusInternal{
					Deployment: &provisioning.ServerDeployment{
						State:          api.ServerDeploymentStateWaitReboot,
						StateEnteredAt: stateEnteredAt,
						InstallSnapshot: provisioning.ServerDeploymentBMCSnapshot{
							Taken:         stateEnteredAt,
							LastResetTime: stateEnteredAt,
						},
					},
				},
			}

			serverSvc := New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				WithNow(func() time.Time { return deploymentTestNow }),
				AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, &adapterMock.BMCServerClientPortMock{}),
			)

			definition := deploymentStates[api.ServerDeploymentStateWaitReboot]
			if tc.noSettleDelay {
				definition.settleDelay = 0
			}

			outcome, _, err := serverSvc.checkDeploymentRebooted(t.Context(), slog.Default(), server, definition)
			require.NoError(t, err, "the wait only observes, the BMC client mock has no function to power the server on with")
			require.Equal(t, tc.wantOutcome, outcome)
		})
	}
}

func Test_serverService_deploymentWait(t *testing.T) {
	const otherWaitRetries = 2

	timedOut := deploymentTestNow.Add(-config.ServerDeploymentStepTimeout - time.Second)

	tests := []struct {
		name           string
		outcome        deploymentWaitOutcome
		waitRetries    int
		stateEnteredAt time.Time

		wantProgressed  bool
		wantState       api.ServerDeploymentState
		wantWaitRetries int
		wantRetries     int
		wantLastError   string
	}{
		{
			name:    "a revert goes back to the step before the wait",
			outcome: deploymentWaitRevert,

			wantProgressed:  true,
			wantState:       api.ServerDeploymentStatePowerOffBIOS,
			wantWaitRetries: 1,
			wantLastError:   "was reverted, since server powered on again",
		},
		{
			name:        "a revert, that exhausts the retry budget, fails the deployment",
			outcome:     deploymentWaitRevert,
			waitRetries: config.ServerDeploymentStepRetries,

			wantState:       api.ServerDeploymentStateFailed,
			wantWaitRetries: config.ServerDeploymentStepRetries,
			wantLastError:   fmt.Sprintf("failed after going back %d times, since server powered on again", config.ServerDeploymentStepRetries),
		},
		{
			name:           "a timeout falls back to the trigger",
			outcome:        deploymentWaitPending,
			stateEnteredAt: timedOut,

			wantProgressed:  true,
			wantState:       api.ServerDeploymentStatePowerOffBIOS,
			wantWaitRetries: 1,
			wantRetries:     1,
			wantLastError:   "did not complete within " + config.ServerDeploymentStepTimeout.String(),
		},
		{
			name:           "a timeout, that exhausts the retry budget, fails the deployment",
			outcome:        deploymentWaitPending,
			waitRetries:    config.ServerDeploymentStepRetries,
			stateEnteredAt: timedOut,

			wantState:       api.ServerDeploymentStateFailed,
			wantWaitRetries: config.ServerDeploymentStepRetries,
			wantLastError:   "did not complete within " + config.ServerDeploymentStepTimeout.String(),
		},
		{
			name:        "a revert spends the last retry of the budget the wait shares with its timeouts",
			outcome:     deploymentWaitRevert,
			waitRetries: config.ServerDeploymentStepRetries - 1,

			wantProgressed:  true,
			wantState:       api.ServerDeploymentStatePowerOffBIOS,
			wantWaitRetries: config.ServerDeploymentStepRetries,
			wantLastError:   "was reverted, since server powered on again",
		},
		{
			name:        "a met wait resets its wait retries",
			outcome:     deploymentWaitMet,
			waitRetries: 1,

			wantProgressed: true,
			wantState:      api.ServerDeploymentStateApplyBIOS,
		},
		{
			name:        "a pending wait keeps the wait retries",
			outcome:     deploymentWaitPending,
			waitRetries: 1,

			wantState:       api.ServerDeploymentStateWaitPowerOffBIOS,
			wantWaitRetries: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			waitRetries := map[api.ServerDeploymentState]int{
				api.ServerDeploymentStateWaitBIOSApplied: otherWaitRetries,
			}

			if tc.waitRetries > 0 {
				waitRetries[api.ServerDeploymentStateWaitPowerOffBIOS] = tc.waitRetries
			}

			stored := provisioning.Server{
				Name:   "one",
				Status: api.ServerStatusDeploying,
				StatusInternal: provisioning.ServerStatusInternal{
					Deployment: &provisioning.ServerDeployment{
						State:          api.ServerDeploymentStateWaitPowerOffBIOS,
						StartedAt:      deploymentTestNow,
						StateEnteredAt: cmp.Or(tc.stateEnteredAt, deploymentTestNow),
						WaitRetries:    waitRetries,
					},
				},
			}

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					server := stored
					deployment := *stored.StatusInternal.Deployment
					deployment.WaitRetries = maps.Clone(deployment.WaitRetries)
					server.StatusInternal.Deployment = &deployment

					return &server, nil
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					stored = in

					return nil
				},
			}

			serverSvc := New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				WithNow(func() time.Time { return deploymentTestNow }),
			)

			definition := deploymentStates[api.ServerDeploymentStateWaitPowerOffBIOS]
			definition.wait = func(*serverService, context.Context, *slog.Logger, provisioning.Server, deploymentStateDefinition) (deploymentWaitOutcome, func(*provisioning.ServerDeployment), error) {
				return tc.outcome, nil, nil
			}

			server, err := repo.GetByName(t.Context(), "one")
			require.NoError(t, err)

			progressed, err := serverSvc.deploymentWait(t.Context(), slog.Default(), *server, definition)
			require.NoError(t, err)

			deployment := stored.StatusInternal.Deployment

			require.Equal(t, tc.wantProgressed, progressed)
			require.Equal(t, tc.wantState, deployment.State)
			require.Equal(t, tc.wantRetries, deployment.Retries, "only a timeout gates the re-issued trigger on the backoff")
			require.Equal(t, otherWaitRetries, deployment.WaitRetries[api.ServerDeploymentStateWaitBIOSApplied], "the wait retries of another wait are left alone")

			if tc.wantWaitRetries == 0 {
				require.NotContains(t, deployment.WaitRetries, api.ServerDeploymentStateWaitPowerOffBIOS)
			} else {
				require.Equal(t, tc.wantWaitRetries, deployment.WaitRetries[api.ServerDeploymentStateWaitPowerOffBIOS])
			}

			if tc.wantLastError == "" {
				require.Empty(t, deployment.LastError)
			} else {
				require.Contains(t, deployment.LastError, tc.wantLastError)
			}
		})
	}
}
