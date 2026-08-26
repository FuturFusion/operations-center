package api_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

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
