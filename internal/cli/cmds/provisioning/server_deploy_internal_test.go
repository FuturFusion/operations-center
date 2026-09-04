package provisioning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestDeploymentProgressLines(t *testing.T) {
	start := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)

	at := func(offset time.Duration) time.Time {
		return start.Add(offset)
	}

	step := func(offset time.Duration, state api.ServerDeploymentState) api.ServerDeploymentStep {
		return api.ServerDeploymentStep{State: state, EnteredAt: at(offset)}
	}

	tests := []struct {
		name         string
		reportedUpTo time.Time
		deployment   api.ServerDeploymentStatus

		wantLines        []string
		wantReportedUpTo time.Time
	}{
		{
			name: "the state of a deployment, that has just been requested",
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStateRefreshBMCData,
				StateEnteredAt: at(0),
			},

			wantLines:        []string{"2026-08-31T10:00:00Z refresh-bmc-data"},
			wantReportedUpTo: at(0),
		},
		{
			name:         "the states, that came and went between two polls",
			reportedUpTo: at(0),
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStateWaitPowerOffBIOS,
				StateEnteredAt: at(20 * time.Second),
				History: []api.ServerDeploymentStep{
					step(0, api.ServerDeploymentStateRefreshBMCData),
					step(10*time.Second, api.ServerDeploymentStatePowerOffBIOS),
				},
			},

			wantLines: []string{
				"2026-08-31T10:00:10Z power-off-bios",
				"2026-08-31T10:00:20Z wait-power-off-bios",
			},
			wantReportedUpTo: at(20 * time.Second),
		},
		{
			name:         "nothing to report, while the deployment stays in the same state",
			reportedUpTo: at(20 * time.Second),
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStateWaitPowerOffBIOS,
				StateEnteredAt: at(20 * time.Second),
				History: []api.ServerDeploymentStep{
					step(0, api.ServerDeploymentStateRefreshBMCData),
					step(10*time.Second, api.ServerDeploymentStatePowerOffBIOS),
				},
			},

			wantLines:        nil,
			wantReportedUpTo: at(20 * time.Second),
		},
		{
			name:         "what a state, that has been reported already, took to complete",
			reportedUpTo: at(20 * time.Second),
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStateApplyBIOS,
				StateEnteredAt: at(5 * time.Minute),
				History: []api.ServerDeploymentStep{
					step(10*time.Second, api.ServerDeploymentStatePowerOffBIOS),
					{
						State:     api.ServerDeploymentStateWaitPowerOffBIOS,
						EnteredAt: at(20 * time.Second),
						Retries:   2,
						Error:     "boom",
					},
				},
			},

			wantLines: []string{
				"  retries: 2",
				"2026-08-31T10:05:00Z apply-bios",
			},
			wantReportedUpTo: at(5 * time.Minute),
		},
		{
			name:         "a state, that reported an error without spending a retry",
			reportedUpTo: at(20 * time.Second),
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStatePowerOffBIOS,
				StateEnteredAt: at(30 * time.Second),
				History: []api.ServerDeploymentStep{
					{
						State:     api.ServerDeploymentStateVerifyBIOS,
						EnteredAt: at(20 * time.Second),
						Error:     `BIOS attributes of server "one" have not been applied: "SecureBoot" is "Disabled" instead of "Enabled"`,
					},
				},
			},

			wantLines: []string{
				"2026-08-31T10:00:30Z power-off-bios",
			},
			wantReportedUpTo: at(30 * time.Second),
		},
		{
			name:         "the states left, after the history has dropped entries",
			reportedUpTo: at(20 * time.Second),
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStateWaitBIOSApplied,
				StateEnteredAt: at(50 * time.Second),
				History: []api.ServerDeploymentStep{
					step(30*time.Second, api.ServerDeploymentStateApplyBIOS),
					step(40*time.Second, api.ServerDeploymentStatePowerOnBIOS),
				},
			},

			wantLines: []string{
				"2026-08-31T10:00:30Z apply-bios",
				"2026-08-31T10:00:40Z power-on-bios",
				"2026-08-31T10:00:50Z wait-bios-applied",
			},
			wantReportedUpTo: at(50 * time.Second),
		},
		{
			name:         "the whole timeline of a deployment, that was already running",
			reportedUpTo: time.Time{},
			deployment: api.ServerDeploymentStatus{
				State:          api.ServerDeploymentStateWaitInstall,
				StateEnteredAt: at(time.Minute),
				History: []api.ServerDeploymentStep{
					step(0, api.ServerDeploymentStateRefreshBMCData),
					step(30*time.Second, api.ServerDeploymentStatePowerOnInstall),
				},
			},

			wantLines: []string{
				"2026-08-31T10:00:00Z refresh-bmc-data",
				"2026-08-31T10:00:30Z power-on-install",
				"2026-08-31T10:01:00Z wait-install",
			},
			wantReportedUpTo: at(time.Minute),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines, reportedUpTo := deploymentProgressLines(tc.deployment, tc.reportedUpTo)

			require.Equal(t, tc.wantLines, lines)
			require.Equal(t, tc.wantReportedUpTo, reportedUpTo)
		})
	}
}

func TestDeploymentProgressLines_reportsEveryStateOnce(t *testing.T) {
	start := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)

	states := []api.ServerDeploymentState{
		api.ServerDeploymentStateRefreshBMCData,
		api.ServerDeploymentStatePowerOffBIOS,
		api.ServerDeploymentStateWaitPowerOffBIOS,
		api.ServerDeploymentStateApplyBIOS,
	}

	var (
		history      = make([]api.ServerDeploymentStep, 0, len(states))
		reported     []string
		reportedUpTo time.Time
	)

	for index, state := range states {
		deployment := api.ServerDeploymentStatus{
			State:          state,
			StateEnteredAt: start.Add(time.Duration(index) * time.Minute),
			History:        history,
		}

		for range 2 {
			var lines []string

			lines, reportedUpTo = deploymentProgressLines(deployment, reportedUpTo)
			reported = append(reported, lines...)
		}

		history = append(history, api.ServerDeploymentStep{
			State:     deployment.State,
			EnteredAt: deployment.StateEnteredAt,
		})
	}

	require.Equal(t, []string{
		"2026-08-31T10:00:00Z refresh-bmc-data",
		"2026-08-31T10:01:00Z power-off-bios",
		"2026-08-31T10:02:00Z wait-power-off-bios",
		"2026-08-31T10:03:00Z apply-bios",
	}, reported)
}
