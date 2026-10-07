package provisioning_test

import (
	"encoding/asn1"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerDeploymentRequest_Validate(t *testing.T) {
	valid := provisioning.ServerDeploymentRequest{
		TokenUUID:    uuid.MustParse("e9de436e-b94e-4aef-8563-883aec84096e"),
		Seed:         "default",
		ImageType:    api.ImageTypeISO,
		Architecture: images.UpdateFileArchitecture64BitX86,
	}

	// The certificates are not parsed as X509, any DER sequence does.
	der, err := asn1.Marshal(struct{ Name string }{Name: "certificate"})
	require.NoError(t, err)

	pemCertificate := certificate.EncodeToPEM(der)

	fingerprint, err := certificate.DERFingerprint("test", []byte(pemCertificate))
	require.NoError(t, err)

	keepingProfile := func(fingerprint string) provisioning.BIOSProfiles {
		return provisioning.BIOSProfiles{{
			Name:  "in-development",
			Match: []provisioning.BIOSProfileMatch{{}},
			SecureBoot: provisioning.BIOSSecureBoot{
				DB: provisioning.BIOSSecureBootDatabase{Certificates: map[string]*bool{fingerprint: new(true)}},
			},
		}}
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
		{
			name: "error - BIOS profile without match",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.BIOSProfiles = provisioning.BIOSProfiles{{Name: "in-development", Attributes: map[string]any{"BootMode": "Uefi"}}}
				return request
			},
			assertErr: errassert.ValidationErrorContains("at least one match is required"),
		},
		{
			name: "valid - secure boot certificate kept by the BIOS profile",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = []string{pemCertificate}

				return request
			},
			assertErr: require.NoError,
		},
		{
			name: "error - secure boot certificate without BIOS profile",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.SecureBootCertificates = []string{pemCertificate}

				return request
			},
			assertErr: errassert.ValidationErrorContains("only be provided together with BIOS profiles"),
		},
		{
			name: "error - secure boot certificate without enrollment media",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = []string{pemCertificate}

				return request
			},
			assertErr: errassert.ValidationErrorContains("only be enrolled from the secure boot enrollment media"),
		},
		{
			name: "error - secure boot certificate not kept by the BIOS profile",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = provisioning.BIOSProfiles{{Name: "in-development", Match: []provisioning.BIOSProfileMatch{{}}, Attributes: map[string]any{"BootMode": "Uefi"}}}
				request.SecureBootCertificates = []string{pemCertificate}

				return request
			},
			assertErr: errassert.ValidationErrorContains("is not kept by any of the provided BIOS profiles"),
		},
		{
			name: "error - secure boot certificate is not PEM encoded",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = []string{"not a certificate"}

				return request
			},
			assertErr: errassert.ValidationErrorContains("has to hold exactly one PEM encoded certificate"),
		},
		{
			name: "error - secure boot certificate file holds more than one certificate",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = []string{pemCertificate + pemCertificate}

				return request
			},
			assertErr: errassert.ValidationErrorContains("has to hold exactly one PEM encoded certificate"),
		},
		{
			name: "error - secure boot certificate is not DER encoded",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = []string{certificate.EncodeToPEM([]byte("certificate"))}

				return request
			},
			assertErr: errassert.ValidationErrorContains("is not DER encoded"),
		},
		{
			name: "error - secure boot certificate provided more than once",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = []string{pemCertificate, pemCertificate}

				return request
			},
			assertErr: errassert.ValidationErrorContains("is provided more than once"),
		},
		{
			name: "error - too many secure boot certificates",
			request: func(request provisioning.ServerDeploymentRequest) provisioning.ServerDeploymentRequest {
				request.SecureBootEnrollmentMedia = true
				request.BIOSProfiles = keepingProfile(fingerprint)
				request.SecureBootCertificates = slices.Repeat([]string{pemCertificate}, 17)

				return request
			},
			assertErr: errassert.ValidationErrorContains("at most 16 secure boot certificates"),
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

	request, err = provisioning.NewServerDeploymentRequest(api.ServerDeploymentPost{
		TokenUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
		Seed:      "default",
		BIOSProfiles: []api.BIOSProfile{{
			Name:       "in-development",
			Match:      []api.BIOSProfileMatch{{Manufacturer: "Lenovo"}},
			Priority:   10,
			Attributes: map[string]any{"BootMode": "Uefi"},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, request.Validate())
	require.Equal(t, "Lenovo", request.BIOSProfiles[0].Match[0].Manufacturer)
	require.Equal(t, 10, request.BIOSProfiles[0].Priority)
	require.Empty(t, request.ToAPI().BIOSProfiles, "The BIOS profiles are not reported back")

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
