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

	require.JSONEq(t, `{"applications":[{"in_maintenance":false,"name":"app","version":"123"}],"os":{"name":"os","version":"123","version_next":"234","needs_reboot":true},"update_channel":"stable"}`, string(val.([]byte)))

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
		needsUpdate   bool
		needsReboot   bool
		inMaintenance bool
		isTypeIncus   bool

		wantServerUpdateState api.ServerUpdateState
	}{
		{
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateReady,
		},
		{
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
		{
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateRebootPending,
		},
		{
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: false,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateEvacuationPending,
		},
		{
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: true,
			isTypeIncus:   true,

			wantServerUpdateState: api.ServerUpdateStateInMaintenanceRebootPending,
		},
		{
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: true,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateInMaintenanceRestorePending,
		},
		{
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
		{
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: true,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
		{
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: true,
			isTypeIncus:   false,

			wantServerUpdateState: api.ServerUpdateStateUpdatePending,
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("needsUpdate: %t, needsReboot: %t, inMaintenance: %t, isTypeIncus: %t", tc.needsUpdate, tc.needsReboot, tc.inMaintenance, tc.isTypeIncus), func(t *testing.T) {
			serverVersionData := api.ServerVersionData{
				NeedsUpdate:   &tc.needsUpdate,
				NeedsReboot:   &tc.needsReboot,
				InMaintenance: &tc.inMaintenance,
			}

			if tc.isTypeIncus {
				serverVersionData.Applications = append(serverVersionData.Applications, api.ApplicationVersionData{
					Name: string(images.UpdateFileComponentIncus),
				})
			}

			got := serverVersionData.State()

			require.Equal(t, tc.wantServerUpdateState, got)
		})
	}
}

func TestServerVersionData_RecommendedAction(t *testing.T) {
	tests := []struct {
		needsUpdate   bool
		needsReboot   bool
		inMaintenance bool
		isTypeIncus   bool

		wantServerAction api.ServerAction
	}{
		{
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionNone,
		},
		{
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
		{
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionReboot,
		},
		{
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: false,
			isTypeIncus:   true,

			wantServerAction: api.ServerActionEvacuate,
		},
		{
			needsUpdate:   false,
			needsReboot:   true,
			inMaintenance: true,
			isTypeIncus:   true,

			wantServerAction: api.ServerActionReboot,
		},
		{
			needsUpdate:   false,
			needsReboot:   false,
			inMaintenance: true,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionRestore,
		},
		{
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: false,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
		{
			needsUpdate:   true,
			needsReboot:   false,
			inMaintenance: true,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
		{
			needsUpdate:   true,
			needsReboot:   true,
			inMaintenance: true,
			isTypeIncus:   false,

			wantServerAction: api.ServerActionUpdate,
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("needsUpdate: %t, needsReboot: %t, inMaintenance: %t, isTypeIncus: %t", tc.needsUpdate, tc.needsReboot, tc.inMaintenance, tc.isTypeIncus), func(t *testing.T) {
			serverVersionData := api.ServerVersionData{
				NeedsUpdate:   &tc.needsUpdate,
				NeedsReboot:   &tc.needsReboot,
				InMaintenance: &tc.inMaintenance,
			}

			if tc.isTypeIncus {
				serverVersionData.Applications = append(serverVersionData.Applications, api.ApplicationVersionData{
					Name: string(images.UpdateFileComponentIncus),
				})
			}

			got := serverVersionData.RecommendedAction()

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
						InMaintenance:    false,
						NeedsUpdate:      ptr.To(false),
					},
				},
				NeedsUpdate:   ptr.To(false),
				NeedsReboot:   ptr.To(false),
				InMaintenance: ptr.To(false),
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
						InMaintenance:    false,
						NeedsUpdate:      ptr.To(true),
					},
				},
				NeedsUpdate:   ptr.To(true),
				NeedsReboot:   ptr.To(false),
				InMaintenance: ptr.To(false),
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
						InMaintenance:    false,
						NeedsUpdate:      ptr.To(false),
					},
				},
				NeedsUpdate:   ptr.To(false),
				NeedsReboot:   ptr.To(true),
				InMaintenance: ptr.To(false),
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
						InMaintenance:    false,
						NeedsUpdate:      ptr.To(true),
					},
				},
				NeedsUpdate:   ptr.To(true),
				NeedsReboot:   ptr.To(false),
				InMaintenance: ptr.To(false),
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
