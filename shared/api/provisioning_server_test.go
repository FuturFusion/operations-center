package api_test

import (
	"fmt"
	"testing"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerVersionData_Value(t *testing.T) {
	svd := api.ServerVersionData{
		OS: api.OSVersionData{
			Name:             "os",
			Version:          "123",
			VersionNext:      "234",
			AvailableVersion: ptr.To("345"),
			NeedsReboot:      true,
			NeedsUpdate:      ptr.To(true),
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:             "app",
				Version:          "123",
				AvailableVersion: ptr.To("234"),
				NeedsUpdate:      ptr.To(true),
			},
		},
		UpdateChannel: "stable",
	}

	val, err := svd.Value()
	require.NoError(t, err)

	require.JSONEq(t, `{"applications":[{"in_maintenance":0,"name":"app","version":"123"}],"os":{"name":"os","version":"123","version_next":"234","needs_reboot":true},"update_channel":"stable"}`, string(val.([]byte)))

	var svdNew api.ServerVersionData
	err = svdNew.Scan(val.([]byte))
	require.NoError(t, err)

	require.Equal(t, api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "os",
			Version:     "123",
			VersionNext: "234",
			NeedsReboot: true,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:    "app",
				Version: "123",
			},
		},
		UpdateChannel: "stable",
	}, svdNew)
}

func TestServerVersionData_State(t *testing.T) {
	tests := []struct {
		status        api.ServerStatus
		statusDetail  api.ServerStatusDetail
		cluster       string
		needsUpdate   bool
		needsReboot   bool
		inMaintenance api.InMaintenanceState
		isTypeIncus   bool

		wantServerUpdateState api.ServerUpdateState
	}{
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpToDate,
		},
		{
			status:        api.ServerStatusOffline,
			statusDetail:  api.ServerStatusDetailOfflineShutdown,
			inMaintenance: api.NotInMaintenance,

			wantServerUpdateState: api.ServerUpdateStateUpToDate,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailReadyUpdating,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdating,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateEvacuationPending,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuating,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateEvacuating,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateInMaintenanceRebootPending,
		},
		{
			status:        api.ServerStatusOffline,
			statusDetail:  api.ServerStatusDetailOfflineRebooting,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateInMaintenanceRebooting,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateInMaintenanceRestorePending,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailReadyRestoring,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.InMaintenanceRestoring,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateInMaintenanceRestoring,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailReadyRestoring,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateInMaintenancePostRestore,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateRebootPending,
		},
		{
			status:        api.ServerStatusOffline,
			statusDetail:  api.ServerStatusDetailOfflineRebooting,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateRebooting,
		},

		// Update pending edge cases:
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},

		// Edge cases:
		{
			status: api.ServerStatusUnknown,

			wantServerUpdateState: api.ServerUpdateStateUndefined,
		},
		{
			status: api.ServerStatusPending,

			wantServerUpdateState: api.ServerUpdateStateUndefined,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailNone,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUndefined,
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("status: %v, statusDetail: %v, cluster: %t, needsUpdate: %t, needsReboot: %t, inMaintenance: %v, isTypeIncus: %t", tc.status, tc.statusDetail, tc.cluster != "", tc.needsUpdate, tc.needsReboot, tc.inMaintenance, tc.isTypeIncus), func(t *testing.T) {
			server := api.Server{
				Status:       tc.status,
				StatusDetail: tc.statusDetail,
				Cluster:      tc.cluster,
				VersionData: api.ServerVersionData{
					NeedsUpdate:   &tc.needsUpdate,
					NeedsReboot:   &tc.needsReboot,
					InMaintenance: &tc.inMaintenance,
				},
			}

			if tc.isTypeIncus {
				server.VersionData.Applications = append(server.VersionData.Applications, api.ApplicationVersionData{
					Name: "incus",
				})
			}

			got := server.UpdateState()

			require.Equal(t, tc.wantServerUpdateState, got)
		})
	}
}

func TestServerVersionData_RecommendedAction(t *testing.T) {
	tests := []struct {
		status        api.ServerStatus
		statusDetail  api.ServerStatusDetail
		cluster       string
		needsUpdate   bool
		needsReboot   bool
		inMaintenance api.InMaintenanceState
		isTypeIncus   bool

		wantServerAction api.ServerAction
	}{
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionNone,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionReboot,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   true,

			wantServerAction: api.ServerActionEvacuate,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   true,

			wantServerAction: api.ServerActionReboot,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "one",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   true,

			wantServerAction: api.ServerActionReboot,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionRestore,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuated,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},

		// Edge cases
		{
			status: api.ServerStatusUnknown,

			wantServerAction: api.ServerActionNone,
		},
		{
			status: api.ServerStatusPending,

			wantServerAction: api.ServerActionNone,
		},
		{
			status: api.ServerStatusOffline,

			wantServerAction: api.ServerActionNone,
		},
		{
			status:        api.ServerStatusReady,
			statusDetail:  api.ServerStatusDetailReadyUpdating,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: api.NotInMaintenance,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionNone,
		},
		{
			status:        api.ServerStatusReady,
			cluster:       "",
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: api.InMaintenanceEvacuating,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionNone,
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("status: %v, statusDetail: %v, cluster: %t, needsUpdate: %t, needsReboot: %t, inMaintenance: %v, isTypeIncus: %t", tc.status, tc.statusDetail, tc.cluster != "", tc.needsUpdate, tc.needsReboot, tc.inMaintenance, tc.isTypeIncus), func(t *testing.T) {
			server := api.Server{
				Status:       tc.status,
				StatusDetail: tc.statusDetail,
				Cluster:      tc.cluster,
				VersionData: api.ServerVersionData{
					NeedsUpdate:   &tc.needsUpdate,
					NeedsReboot:   &tc.needsReboot,
					InMaintenance: &tc.inMaintenance,
				},
			}

			if tc.isTypeIncus {
				server.VersionData.Applications = append(server.VersionData.Applications, api.ApplicationVersionData{
					Name: "incus",
				})
			}

			got := server.RecommendedAction()

			require.Equal(t, tc.wantServerAction, got)
		})
	}
}

func TestServerVersionData_Compute(t *testing.T) {
	tests := []struct {
		name                    string
		serverVersionData       api.ServerVersionData
		latestAvailableVersions map[images.UpdateFileComponent]string

		wantServerVersionData api.ServerVersionData
	}{
		{
			name: "everything up to date",
			serverVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "202602230000",
					VersionNext: "",
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:    "incus",
						Version: "202602230000",
					},
				},
			},
			latestAvailableVersions: map[images.UpdateFileComponent]string{
				images.UpdateFileComponentOS:    "202602230000",
				images.UpdateFileComponentIncus: "202602230000",
			},
			wantServerVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:             "incusos",
					Version:          "202602230000",
					VersionNext:      "",
					NeedsReboot:      false,
					AvailableVersion: ptr.To("202602230000"),
					NeedsUpdate:      ptr.To(false),
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:             "incus",
						Version:          "202602230000",
						AvailableVersion: ptr.To("202602230000"),
						InMaintenance:    api.NotInMaintenance,
						NeedsUpdate:      ptr.To(false),
					},
				},
				NeedsUpdate:   ptr.To(false),
				NeedsReboot:   ptr.To(false),
				InMaintenance: ptr.To(api.NotInMaintenance),
			},
		},
		{
			name: "os and app needs update",
			serverVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "202602230000",
					VersionNext: "",
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:    "incus",
						Version: "202602230000",
					},
				},
			},
			latestAvailableVersions: map[images.UpdateFileComponent]string{
				images.UpdateFileComponentOS:    "202602230001",
				images.UpdateFileComponentIncus: "202602230001",
			},
			wantServerVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:             "incusos",
					Version:          "202602230000",
					VersionNext:      "",
					NeedsReboot:      false,
					AvailableVersion: ptr.To("202602230001"),
					NeedsUpdate:      ptr.To(true),
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:             "incus",
						Version:          "202602230000",
						AvailableVersion: ptr.To("202602230001"),
						InMaintenance:    api.NotInMaintenance,
						NeedsUpdate:      ptr.To(true),
					},
				},
				NeedsUpdate:   ptr.To(true),
				NeedsReboot:   ptr.To(false),
				InMaintenance: ptr.To(api.NotInMaintenance),
			},
		},
		{
			name: "os has current version as next",
			serverVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "202602230000",
					VersionNext: "202602230001",
					NeedsReboot: true,
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:    "incus",
						Version: "202602230001",
					},
				},
			},
			latestAvailableVersions: map[images.UpdateFileComponent]string{
				images.UpdateFileComponentOS:    "202602230001",
				images.UpdateFileComponentIncus: "202602230001",
			},
			wantServerVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:             "incusos",
					Version:          "202602230000",
					VersionNext:      "202602230001",
					NeedsReboot:      true,
					AvailableVersion: ptr.To("202602230001"),
					NeedsUpdate:      ptr.To(false),
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:             "incus",
						Version:          "202602230001",
						AvailableVersion: ptr.To("202602230001"),
						InMaintenance:    api.NotInMaintenance,
						NeedsUpdate:      ptr.To(false),
					},
				},
				NeedsUpdate:   ptr.To(false),
				NeedsReboot:   ptr.To(true),
				InMaintenance: ptr.To(api.NotInMaintenance),
			},
		},
		{
			name: "app needs update",
			serverVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:        "incusos",
					Version:     "202602230000",
					VersionNext: "",
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:    "incus",
						Version: "202602230000",
					},
				},
			},
			latestAvailableVersions: map[images.UpdateFileComponent]string{
				images.UpdateFileComponentOS:    "202602230000",
				images.UpdateFileComponentIncus: "202602230001",
			},
			wantServerVersionData: api.ServerVersionData{
				OS: api.OSVersionData{
					Name:             "incusos",
					Version:          "202602230000",
					VersionNext:      "",
					NeedsReboot:      false,
					AvailableVersion: ptr.To("202602230000"),
					NeedsUpdate:      ptr.To(false),
				},
				Applications: []api.ApplicationVersionData{
					{
						Name:             "incus",
						Version:          "202602230000",
						AvailableVersion: ptr.To("202602230001"),
						InMaintenance:    api.NotInMaintenance,
						NeedsUpdate:      ptr.To(true),
					},
				},
				NeedsUpdate:   ptr.To(true),
				NeedsReboot:   ptr.To(false),
				InMaintenance: ptr.To(api.NotInMaintenance),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.serverVersionData
			got.Compute(tc.latestAvailableVersions)

			require.Equal(t, tc.wantServerVersionData, got)
		})
	}
}
