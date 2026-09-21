package api_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestBMCBootProgressHandedOverToOS(t *testing.T) {
	tests := []struct {
		name  string
		state string

		want bool
	}{
		{name: "no state reported", state: "", want: false},
		{name: "the firmware is still initializing", state: "PrimaryProcessorInitializationStarted", want: false},
		{name: "the last state before the hand over", state: "SetupEntered", want: false},
		{name: "the operating system is being started", state: "OSBootStarted", want: true},
		{name: "the operating system is running", state: "OSRunning", want: true},
		{name: "a state carrying no ordering information", state: "OEM", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, api.BMCBootProgressHandedOverToOS(tc.state), "only a boot, that got as far as starting the operating system, has run the installer")
		})
	}
}

func TestBMCHasRebootedSince(t *testing.T) {
	since := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		previous api.BMCData
		current  api.BMCData

		want api.BMCRebootState
	}{
		{
			name:     "unknown - BMC reports neither property",
			previous: api.BMCData{ServerPowerState: "On"},
			current:  api.BMCData{ServerPowerState: "On"},

			want: api.BMCRebootStateUnknown,
		},
		{
			name: "unknown - last reset time went backwards",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
			},
			current: api.BMCData{
				ServerLastResetTime: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			},

			want: api.BMCRebootStateUnknown,
		},
		{
			name: "not rebooted - last reset time did not advance",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
			},
			current: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name: "not rebooted - boot progress did not move",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name: "rebooted - last reset time advanced past since",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
			},
			current: api.BMCData{
				ServerLastResetTime: since.Add(time.Minute),
			},

			want: api.BMCRebootStateRebooted,
		},
		{
			name: "rebooted - boot progress state time moved past since",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(time.Minute),
				},
			},

			want: api.BMCRebootStateRebooted,
		},
		{
			name: "rebooted - boot progress fell back from a late to an early state",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "MemoryInitializationStarted",
					LastStateTime: since.Add(-2 * time.Hour),
				},
			},

			want: api.BMCRebootStateRebooted,
		},
		{
			name: "not rebooted - boot progress advanced during the same boot",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "MemoryInitializationStarted",
					LastStateTime: since.Add(-2 * time.Hour),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name: "not rebooted - boot progress advanced past since during the same boot",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "MemoryInitializationStarted",
					LastStateTime: since.Add(-time.Minute),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "PCIResourceConfigStarted",
					LastStateTime: since.Add(time.Minute),
				},
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name: "rebooted - boot progress fell back past since",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "PCIResourceConfigStarted",
					LastStateTime: since.Add(-time.Minute),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "MemoryInitializationStarted",
					LastStateTime: since.Add(time.Minute),
				},
			},

			want: api.BMCRebootStateRebooted,
		},
		{
			name: "not rebooted - boot progress state can not be placed in a boot",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OEM",
					LastStateTime: since.Add(-time.Minute),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OEM",
					LastStateTime: since.Add(time.Minute),
				},
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name:     "unknown - last reset time only reported by the previous snapshot",
			previous: api.BMCData{ServerLastResetTime: since.Add(-time.Hour)},
			current:  api.BMCData{},

			want: api.BMCRebootStateUnknown,
		},
		{
			name: "unknown - boot progress state without a state time",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState: "OSRunning",
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState: "OSRunning",
				},
			},

			want: api.BMCRebootStateUnknown,
		},
		{
			name: "rebooted - boot progress regressed without a state time",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState: "OSRunning",
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState: "MemoryInitializationStarted",
				},
			},

			want: api.BMCRebootStateRebooted,
		},
		{
			name: "not rebooted - last reset time did not advance despite being after since",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(time.Hour),
			},
			current: api.BMCData{
				ServerLastResetTime: since.Add(time.Hour),
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name: "rebooted - last reset time advanced while the BMC clock runs ahead",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(time.Hour),
			},
			current: api.BMCData{
				ServerLastResetTime: since.Add(2 * time.Hour),
			},

			want: api.BMCRebootStateRebooted,
		},
		{
			name: "unknown - boot progress state time went backwards",
			previous: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},
			current: api.BMCData{
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-2 * time.Hour),
				},
			},

			want: api.BMCRebootStateUnknown,
		},
		{
			name: "not rebooted - boot progress state time went backwards and last reset time did not advance",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-time.Hour),
				},
			},
			current: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
				ServerBootProgress: api.BMCBootProgress{
					LastState:     "OSRunning",
					LastStateTime: since.Add(-2 * time.Hour),
				},
			},

			want: api.BMCRebootStateNotRebooted,
		},
		{
			name: "not rebooted - boot progress without a state time and last reset time did not advance",
			previous: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
				ServerBootProgress: api.BMCBootProgress{
					LastState: "OSRunning",
				},
			},
			current: api.BMCData{
				ServerLastResetTime: since.Add(-time.Hour),
				ServerBootProgress: api.BMCBootProgress{
					LastState: "OSRunning",
				},
			},

			want: api.BMCRebootStateNotRebooted,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := api.BMCHasRebootedSince(tc.previous, tc.current, since)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestBMCData_Missing(t *testing.T) {
	tests := []struct {
		name string
		data api.BMCData
		ask  []api.BMCDataPart

		want []api.BMCDataPart
	}{
		{
			name: "everything has been collected",
			data: api.BMCData{},
			ask:  []api.BMCDataPart{api.BMCDataPartVirtualMedia},

			want: nil,
		},
		{
			name: "the part asked for is unavailable",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
			}},
			ask: []api.BMCDataPart{api.BMCDataPartVirtualMedia},

			want: []api.BMCDataPart{api.BMCDataPartVirtualMedia},
		},
		{
			name: "another part is unavailable",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartBIOSAttributes: "BMC returned HTTP 503",
			}},
			ask: []api.BMCDataPart{api.BMCDataPartVirtualMedia},

			want: nil,
		},
		{
			name: "several parts are unavailable and come back in collection order",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
				api.BMCDataPartSystem:       "BMC returned HTTP 503",
			}},
			ask: []api.BMCDataPart{api.BMCDataPartVirtualMedia, api.BMCDataPartSystem},

			want: []api.BMCDataPart{api.BMCDataPartSystem, api.BMCDataPartVirtualMedia},
		},
		{
			name: "nothing is asked for",
			data: api.BMCData{Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
			}},
			ask: nil,

			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.data.Missing(tc.ask...))
		})
	}
}

func TestBMCData_DescribeMissing(t *testing.T) {
	data := api.BMCData{Unavailable: map[api.BMCDataPart]string{
		api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
	}}

	require.Equal(t, "virtual_media (BMC returned HTTP 503)", data.DescribeMissing(api.BMCDataPartVirtualMedia))
	require.Equal(t, "nothing", data.DescribeMissing(api.BMCDataPartSystem))
	require.Equal(t, "nothing", api.BMCData{}.DescribeMissing(api.BMCDataPartVirtualMedia))
}

// TestBMCDataParts_areComplete guards against a part being added to the
// constants without being added to BMCDataParts, which Missing iterates and
// which would silently never report the new part as missing.
func TestBMCDataParts_areComplete(t *testing.T) {
	declared := []api.BMCDataPart{
		api.BMCDataPartSystem,
		api.BMCDataPartManager,
		api.BMCDataPartProcessor,
		api.BMCDataPartTrustedModules,
		api.BMCDataPartBIOSAttributes,
		api.BMCDataPartVirtualMedia,
	}

	require.ElementsMatch(t, declared, api.BMCDataParts)

	for _, part := range api.BMCDataParts {
		t.Run(part.String(), func(t *testing.T) {
			require.NotEmpty(t, part.String())

			data := api.BMCData{Unavailable: map[api.BMCDataPart]string{part: "boom"}}
			require.Equal(t, []api.BMCDataPart{part}, data.Missing(part))
		})
	}
}

func TestBMCData_CarryOver(t *testing.T) {
	previous := api.BMCData{
		BMCModel:         "iDRAC9",
		ServerModel:      "PowerEdge R770",
		ServerPowerState: "On",
		ServerHasTPM:     true,
		ServerBIOSAttributes: map[string]any{
			"BootMode": "Uefi",
		},
		VirtualMedia: map[string]api.BMCVirtualMedia{
			"system:1": {ID: "system:1", Inserted: true},
		},
	}

	t.Run("a part, that was collected, keeps what was collected", func(t *testing.T) {
		current := api.BMCData{ServerPowerState: "Off"}

		got := current.CarryOver(previous)

		require.Equal(t, "Off", got.ServerPowerState)
		require.Nil(t, got.VirtualMedia, "nothing is inherited without a part being named")
	})

	t.Run("a part, that could not be collected, keeps what was observed before", func(t *testing.T) {
		current := api.BMCData{
			ServerPowerState: "Off",
			Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
			},
		}

		got := current.CarryOver(previous)

		require.Equal(t, previous.VirtualMedia, got.VirtualMedia)
		require.Equal(t, "Off", got.ServerPowerState, "a part, that was collected, is not overwritten")
		require.Equal(t, current.Unavailable, got.Unavailable, "the part stays named as not collected")
	})

	t.Run("nothing to inherit leaves the part at its zero value", func(t *testing.T) {
		current := api.BMCData{
			Unavailable: map[api.BMCDataPart]string{
				api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
			},
		}

		got := current.CarryOver(api.BMCData{})

		require.Nil(t, got.VirtualMedia)
	})
}

func TestBMCData_CarryOverCoversEveryField(t *testing.T) {
	previous := api.BMCData{
		BMCModel:                      "iDRAC9",
		BMCFirmwareVersion:            "1.30.20.10",
		BMCServiceIdentification:      "ServiceID1",
		ServerManufacturer:            "Dell Inc.",
		ServerModel:                   "PowerEdge R770",
		ServerSubModel:                "SubModel",
		ServerUUID:                    "e9de436e-b94e-4aef-8563-883aec84096e",
		ServerAssetTag:                "AssetTag1",
		ServerHostName:                "host1",
		ServerSKU:                     "SKU123",
		ServerSerialNumber:            "Serial123",
		ServerBIOSVersion:             "1.7.5",
		ServerBIOSAttributes:          map[string]any{"BootMode": "Uefi"},
		ServerProcessorManufacturer:   "Intel",
		ServerProcessorArchitecture:   "x86",
		ServerProcessorInstructionSet: "x86-64",
		ServerCPUSockets:              2,
		ServerHasTPM:                  true,
		ServerPowerState:              "On",
		ServerLocationIndicatorActive: true,
		ServerHealthStatus:            "OK",
		ServerLastResetTime:           time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
		ServerBootProgress:            api.BMCBootProgress{LastState: "OSRunning"},
		VirtualMedia: map[string]api.BMCVirtualMedia{
			"system:1": {ID: "system:1", Inserted: true},
		},
	}

	unavailable := map[api.BMCDataPart]string{}
	for _, part := range api.BMCDataParts {
		unavailable[part] = "BMC returned HTTP 503"
	}

	// The protocol fields come off the connection rather than off a part, so
	// they are what an unreachable BMC still reports about itself.
	current := api.BMCData{
		BMCProtocol:        "Redfish",
		BMCProtocolVersion: "1.16.0",
		BMCVendor:          "Dell",
		Unavailable:        unavailable,
	}

	got := current.CarryOver(previous)

	want := previous
	want.BMCProtocol = "Redfish"
	want.BMCProtocolVersion = "1.16.0"
	want.BMCVendor = "Dell"
	want.Unavailable = unavailable

	require.Equal(t, want, got)
}
