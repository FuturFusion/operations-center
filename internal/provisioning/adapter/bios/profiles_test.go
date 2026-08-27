package bios_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/bios"
	"github.com/FuturFusion/operations-center/shared/api"
)

func dellPowerEdgeR740(mutators ...func(data *api.BMCData)) api.BMCData {
	data := api.BMCData{
		BMCProtocol:                   "Redfish",
		BMCProtocolVersion:            "1.17.0",
		BMCVendor:                     "Dell",
		BMCModel:                      "14G Monolithic",
		BMCFirmwareVersion:            "7.00.00.184",
		ServerManufacturer:            "Dell Inc.",
		ServerModel:                   "PowerEdge R740xd",
		ServerUUID:                    "4c4c4544-0043-4a10-8054-c8c04f4a3532",
		ServerBIOSVersion:             "2.27.0",
		ServerProcessorManufacturer:   "Intel",
		ServerProcessorArchitecture:   "x86",
		ServerProcessorInstructionSet: "x86-64",
		ServerCPUSockets:              2,
		ServerHasTPM:                  true,
		ServerPowerState:              "On",
		ServerHealthStatus:            "OK",
	}

	for _, mutator := range mutators {
		mutator(&data)
	}

	return data
}

// TestCatalogue_builtinProfiles resolves the BIOS profiles shipped with
// Operations Center against the BMC data of the hardware they have been written
// for. Every case states the complete resolution, so a reorganization of the
// profiles, that changes the profiles applied to a server or the values they
// assign, is caught here.
func TestCatalogue_builtinProfiles(t *testing.T) {
	tests := []struct {
		name string
		data api.BMCData

		want *provisioning.BIOSProfileResolution
	}{
		{
			name: "Dell PowerEdge R740 with TPM",
			data: dellPowerEdgeR740(),

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"dell", "dell-with-tpm"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootMode":   "UserMode",
					"SecureBootPolicy": "Custom",
					"TpmSecurity":      "On",
				},
			},
		},
		{
			name: "Dell PowerEdge R740 without TPM does not get TpmSecurity",
			data: dellPowerEdgeR740(func(data *api.BMCData) {
				data.ServerHasTPM = false
			}),

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"dell"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootMode":   "UserMode",
					"SecureBootPolicy": "Custom",
				},
			},
		},
		{
			name: "manufacturer reported with surrounding whitespace still matches",
			data: dellPowerEdgeR740(func(data *api.BMCData) {
				data.ServerManufacturer = " Dell Inc. "
			}),

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"dell", "dell-with-tpm"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootMode":   "UserMode",
					"SecureBootPolicy": "Custom",
					"TpmSecurity":      "On",
				},
			},
		},
		{
			name: "server of another manufacturer with TPM has no profile",
			data: dellPowerEdgeR740(func(data *api.BMCData) {
				data.BMCVendor = "Supermicro"
				data.BMCModel = "X11DPU"
				data.ServerManufacturer = "Supermicro"
				data.ServerModel = "SYS-1029U-TRTP"
			}),

			want: nil,
		},
		{
			name: "server without BMC data has no profile",
			data: api.BMCData{},

			want: nil,
		},
	}

	catalogue, err := bios.New()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolution, err := catalogue.Resolve(context.Background(), provisioning.Server{
				Name:    "one",
				BMCData: tc.data,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, resolution)
		})
	}
}
