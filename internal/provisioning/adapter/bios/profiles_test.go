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
				DeferredAttributes: map[string]any{
					"Tpm2Algorithm": "SHA256",
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
				DeferredAttributes: map[string]any{},
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
				DeferredAttributes: map[string]any{
					"Tpm2Algorithm": "SHA256",
				},
			},
		},
		{
			name: "Lenovo ThinkSystem SR630 V2",
			data: api.BMCData{
				BMCProtocol:                   "Redfish",
				BMCProtocolVersion:            "1.15.0",
				BMCVendor:                     "Lenovo",
				BMCModel:                      "Lenovo XClarity Controller",
				BMCFirmwareVersion:            "AFBT58B 5.70 2025-08-11",
				ServerManufacturer:            "Lenovo",
				ServerModel:                   "ThinkSystem SR630 V2",
				ServerUUID:                    "8a1f2b3c-4d5e-6f70-8192-a3b4c5d6e7f8",
				ServerBIOSVersion:             "AFE128F",
				ServerProcessorManufacturer:   "Intel",
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
				ServerCPUSockets:              1,
				ServerHasTPM:                  true,
				ServerPowerState:              "On",
				ServerHealthStatus:            "OK",
			},

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"lenovo"},
				Attributes: map[string]any{
					"SecureBootConfiguration_SecureBootStatus": "Enabled",
					"SecureBootConfiguration_SecureBootMode":   "UserMode",
					"SecureBootConfiguration_SecureBootPolicy": "CustomPolicy",
				},
				DeferredAttributes: map[string]any{},
			},
		},
		{
			name: "HPE ProLiant DL385 Gen10",
			data: api.BMCData{
				BMCProtocol:                   "Redfish",
				BMCProtocolVersion:            "1.20.0",
				BMCVendor:                     "HPE",
				BMCModel:                      "iLO 5",
				BMCFirmwareVersion:            "iLO 5 v3.20",
				ServerManufacturer:            "HPE",
				ServerModel:                   "ProLiant DL385 Gen10",
				ServerUUID:                    "1b2c3d4e-5f60-7182-93a4-b5c6d7e8f901",
				ServerBIOSVersion:             "A40 v3.70 (01/09/2026)",
				ServerProcessorManufacturer:   "Advanced Micro Devices, Inc.",
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
				ServerCPUSockets:              2,
				ServerHasTPM:                  true,
				ServerPowerState:              "On",
				ServerHealthStatus:            "OK",
			},

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"hp", "hp-with-amd"},
				Attributes: map[string]any{
					"NumaGroupSizeOpt": "Clustered",
					"SecureBootStatus": "Enabled",
				},
				DeferredAttributes: map[string]any{},
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

	catalog, err := bios.New()
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolution, err := catalog.Resolve(context.Background(), provisioning.Server{
				Name:    "one",
				BMCData: tc.data,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, resolution)
		})
	}
}
