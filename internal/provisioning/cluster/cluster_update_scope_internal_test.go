package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestRollingUpdateScope(t *testing.T) {
	osTrigger := api.ServerUpdateApplication{
		Name:          "os",
		TriggerUpdate: true,
	}

	tests := []struct {
		name                 string
		inProgressStatus     api.ClusterUpdateInProgressStatus
		osNeedsUpdate        bool
		incusNeedsUpdate     bool
		incusCephNeedsUpdate bool

		wantNeedsUpdate bool
		wantRequest     api.ServerUpdatePost
	}{
		{
			name:             "unrestricted - application outdated",
			incusNeedsUpdate: true,

			wantNeedsUpdate: true,
			wantRequest:     api.ServerUpdatePost{OS: osTrigger},
		},
		{
			name:             "os only - OS outdated",
			inProgressStatus: api.ClusterUpdateInProgressStatus{OSOnly: true},
			osNeedsUpdate:    true,
			incusNeedsUpdate: true,

			wantNeedsUpdate: true,
			wantRequest:     api.ServerUpdatePost{OS: osTrigger, OSOnly: true},
		},
		{
			name:             "os only - only application outdated",
			inProgressStatus: api.ClusterUpdateInProgressStatus{OSOnly: true},
			incusNeedsUpdate: true,

			wantNeedsUpdate: false,
		},
		{
			name:                 "applications - covered application outdated",
			inProgressStatus:     api.ClusterUpdateInProgressStatus{Applications: []string{"incus"}},
			osNeedsUpdate:        true,
			incusNeedsUpdate:     true,
			incusCephNeedsUpdate: true,

			wantNeedsUpdate: true,
			wantRequest: api.ServerUpdatePost{
				Applications: []api.ServerUpdateApplication{
					{
						Name:          "incus",
						TriggerUpdate: true,
					},
				},
			},
		},
		{
			name:                 "applications - only OS and other application outdated",
			inProgressStatus:     api.ClusterUpdateInProgressStatus{Applications: []string{"incus"}},
			osNeedsUpdate:        true,
			incusCephNeedsUpdate: true,

			wantNeedsUpdate: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := provisioning.Server{
				Status: api.ServerStatusReady,
				VersionData: api.ServerVersionData{
					OS: api.OSVersionData{
						NeedsUpdate: new(tc.osNeedsUpdate),
					},
					Applications: []api.ApplicationVersionData{
						{
							Name:        "incus",
							NeedsUpdate: new(tc.incusNeedsUpdate),
						},
						{
							Name:        "incus-ceph",
							NeedsUpdate: new(tc.incusCephNeedsUpdate),
						},
					},
					NeedsUpdate: new(tc.osNeedsUpdate || tc.incusNeedsUpdate || tc.incusCephNeedsUpdate),
				},
			}

			require.Equal(t, tc.wantNeedsUpdate, serverNeedsUpdateInScope(tc.inProgressStatus, server))

			// The progress only counts the server as pending for what the run covers.
			inProgressStatus := tc.inProgressStatus
			inProgressStatus.InProgress = api.ClusterUpdateInProgressApplyUpdate
			updatePending := serverUpdateStateForRollingUpdate(inProgressStatus, server) == api.ServerUpdateStateUpdatePending
			require.Equal(t, tc.wantNeedsUpdate, updatePending)

			// The request is only built for a server, which needs an update.
			if tc.wantNeedsUpdate {
				require.Equal(t, tc.wantRequest, rollingUpdateRequest(tc.inProgressStatus, server))
			}
		})
	}
}
