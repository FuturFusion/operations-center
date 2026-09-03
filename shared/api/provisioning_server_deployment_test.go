package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerDeploymentState_UnmarshalText(t *testing.T) {
	tests := []struct {
		name string
		text string

		want      api.ServerDeploymentState
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:      "valid state",
			text:      "wait-install",
			want:      api.ServerDeploymentStateWaitInstall,
			assertErr: require.NoError,
		},
		{
			name:      "valid state of the second BIOS pass",
			text:      "apply-bios-deferred",
			want:      api.ServerDeploymentStateApplyBIOSDeferred,
			assertErr: require.NoError,
		},
		{
			name:      "empty state",
			text:      "",
			want:      "",
			assertErr: require.NoError,
		},
		{
			name:      "invalid state",
			text:      "not-a-state",
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var state api.ServerDeploymentState

			err := state.UnmarshalText([]byte(tc.text))
			tc.assertErr(t, err)

			if err != nil {
				return
			}

			require.Equal(t, tc.want, state)
		})
	}
}

func TestServerDeploymentStatus_roundTrip(t *testing.T) {
	status := api.ServerDeploymentStatus{
		State:       api.ServerDeploymentStateWaitInstall,
		FailedState: "",
		Request: api.ServerDeploymentPost{
			TokenUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			Seed:      "default",
		},
		BIOSAttributes: map[string]any{
			"TpmSecurity": "On",
		},
		BIOSDeferredAttributes: map[string]any{
			"Tpm2Algorithm": "SHA256",
		},
		Retries: 2,
		History: []api.ServerDeploymentStep{
			{State: api.ServerDeploymentStateAttachMedia},
			{State: api.ServerDeploymentStateVerifyBIOSDeferred},
		},
	}

	encoded, err := json.Marshal(status)
	require.NoError(t, err)

	var decoded api.ServerDeploymentStatus

	err = json.Unmarshal(encoded, &decoded)
	require.NoError(t, err)

	require.Equal(t, status, decoded)
}

func TestServerDeploymentState_IsTerminal(t *testing.T) {
	terminal := []api.ServerDeploymentState{
		api.ServerDeploymentStateCompleted,
		api.ServerDeploymentStateFailed,
		api.ServerDeploymentStateCancelled,
	}

	for _, state := range terminal {
		require.Truef(t, state.IsTerminal(), "expected %q to be terminal", state)
	}

	ongoing := []api.ServerDeploymentState{
		api.ServerDeploymentStateRefreshBMCData,
		api.ServerDeploymentStatePowerOnSecureBoot,
		api.ServerDeploymentStateWaitSecureBootSettled,
		api.ServerDeploymentStatePowerOffSecureBootSettled,
		api.ServerDeploymentStateWaitPowerOffSecureBootSettled,
		api.ServerDeploymentStateWaitInstall,
		api.ServerDeploymentStateCancel,
		api.ServerDeploymentStateWaitCancel,
	}

	for _, state := range ongoing {
		require.Falsef(t, state.IsTerminal(), "expected %q not to be terminal", state)
	}
}
