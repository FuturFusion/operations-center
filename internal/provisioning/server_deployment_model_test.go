package provisioning_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerDeploymentRequest_Validate(t *testing.T) {
	valid := provisioning.ServerDeploymentRequest{
		TokenUUID:    uuid.MustParse("e9de436e-b94e-4aef-8563-883aec84096e"),
		Seed:         "default",
		ImageType:    api.ImageTypeISO,
		Architecture: images.UpdateFileArchitecture64BitX86,
	}

	tests := []struct {
		name    string
		request func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				return request
			},
			assertErr: require.NoError,
		},
		{
			name: "error - empty token",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.TokenUUID = uuid.Nil
				return request
			},
			assertErr: require.Error,
		},
		{
			name: "error - empty seed",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.Seed = ""
				return request
			},
			assertErr: require.Error,
		},
		{
			name: "error - invalid image type",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.ImageType = "qcow2"
				return request
			},
			assertErr: require.Error,
		},
		{
			name: "success - undefined architecture",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.Architecture = images.UpdateFileArchitectureUndefined
				return request
			},
			assertErr: require.NoError,
		},
		{
			name: "error - unknown architecture",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.Architecture = "ppc64le"
				return request
			},
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assertErr(t, tc.request(valid).Validate())
		})
	}
}

func TestNewServerDeploymentRequest(t *testing.T) {
	request, err := provisioning.NewServerDeploymentRequest(api.ServerDeploymentPost{
		TokenUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
		Seed:      "default",
	})
	require.NoError(t, err)

	require.Equal(t, api.ImageTypeISO, request.ImageType)
	require.Equal(t, images.UpdateFileArchitectureUndefined, request.Architecture,
		"An architecture, that was not asked for, stays empty, so DeployByName takes it from the BMC of the server")
	require.False(t, request.SkipSecureBootCertificates)
	require.NoError(t, request.Validate())

	request, err = provisioning.NewServerDeploymentRequest(api.ServerDeploymentPost{
		TokenUUID:                  "e9de436e-b94e-4aef-8563-883aec84096e",
		Seed:                       "default",
		SkipSecureBootCertificates: true,
	})
	require.NoError(t, err)
	require.True(t, request.SkipSecureBootCertificates)
	require.True(t, request.ToAPI().SkipSecureBootCertificates)

	_, err = provisioning.NewServerDeploymentRequest(api.ServerDeploymentPost{
		TokenUUID: "not-a-uuid",
		Seed:      "default",
	})
	require.Error(t, err)
}

func TestServerDeployment_IsActive(t *testing.T) {
	var missing *provisioning.ServerDeployment

	require.False(t, missing.IsActive())
	require.True(t, (&provisioning.ServerDeployment{State: api.ServerDeploymentStateWaitInstall}).IsActive())
	require.False(t, (&provisioning.ServerDeployment{State: api.ServerDeploymentStateCompleted}).IsActive())
}

func TestServerDeployment_ToAPI(t *testing.T) {
	deployment := provisioning.ServerDeployment{
		State:        api.ServerDeploymentStateVerifyBIOS,
		BIOSProfiles: []string{"dell", "dell-with-tpm"},
		BIOSAttributes: map[string]any{
			"TpmSecurity": "On",
		},
		BIOSDeferredAttributes: map[string]any{
			"Tpm2Algorithm": "SHA256",
		},
	}

	status := deployment.ToAPI()

	require.Equal(t, api.ServerDeploymentStateVerifyBIOS, status.State)
	require.Equal(t, []string{"dell", "dell-with-tpm"}, status.BIOSProfiles)
	require.Equal(t, map[string]any{"TpmSecurity": "On"}, status.BIOSAttributes)
	require.Equal(t, map[string]any{"Tpm2Algorithm": "SHA256"}, status.BIOSDeferredAttributes)
}

func TestServerDeployment_EnterState(t *testing.T) {
	entered := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)

	deployment := provisioning.ServerDeployment{
		State:          api.ServerDeploymentStateAttachMedia,
		StateEnteredAt: entered,
		Retries:        2,
		LastError:      "boom",
	}

	deployment.EnterState(entered.Add(time.Minute), api.ServerDeploymentStateWaitMediaAttached)

	require.Equal(t, api.ServerDeploymentStateWaitMediaAttached, deployment.State)
	require.Equal(t, entered.Add(time.Minute), deployment.StateEnteredAt)
	require.Zero(t, deployment.Retries)
	require.Empty(t, deployment.LastError)
	require.Equal(t, []api.ServerDeploymentStep{
		{
			State:     api.ServerDeploymentStateAttachMedia,
			EnteredAt: entered,
			Retries:   2,
			Error:     "boom",
		},
	}, deployment.History)

	deployment.Retries = 1
	deployment.LastError = "timeout"

	deployment.FallBackTo(entered.Add(2*time.Minute), api.ServerDeploymentStateAttachMedia)

	require.Equal(t, api.ServerDeploymentStateAttachMedia, deployment.State)
	require.Equal(t, 1, deployment.Retries)
	require.Equal(t, "timeout", deployment.LastError)

	deployment.EnterState(entered.Add(3*time.Minute), api.ServerDeploymentStateFailed)

	require.Equal(t, entered.Add(3*time.Minute), deployment.FinishedAt)
}

func TestServerDeploymentBMCSnapshot(t *testing.T) {
	taken := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	reset := taken.Add(-time.Hour)

	snapshot := provisioning.NewServerDeploymentBMCSnapshot(taken, api.BMCData{
		ServerLastResetTime: reset,
		ServerBootProgress:  api.BMCBootProgress{LastState: "OSRunning", LastStateTime: reset},
		ServerPowerState:    "On",
	})

	require.Equal(t, taken, snapshot.Taken)
	require.Equal(t, api.BMCData{
		ServerLastResetTime: reset,
		ServerBootProgress:  api.BMCBootProgress{LastState: "OSRunning", LastStateTime: reset},
	}, snapshot.BMCData())
}
