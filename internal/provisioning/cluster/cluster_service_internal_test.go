package cluster

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_determineManagementAddress(t *testing.T) {
	tests := []struct {
		name      string
		serverArg provisioning.Server

		want string
	}{
		{
			name: "from management role",
			serverArg: provisioning.Server{
				ConnectionURL: "https://10.10.10.10:8443",
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{
										"192.168.0.100",
									},
									Roles: []string{
										"management",
									},
								},
							},
						},
					},
				},
			},

			want: "192.168.0.100:8443",
		},
		{
			name: "without management role",
			serverArg: provisioning.Server{
				ConnectionURL: "https://10.10.10.10:8443",
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{
										"192.168.0.100",
									},
									Roles: []string{}, // management role missing
								},
							},
						},
					},
				},
			},

			want: ":8443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := determineManagementRoleAddress(tc.serverArg)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_determineClusterAddress(t *testing.T) {
	tests := []struct {
		name      string
		serverArg provisioning.Server

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name: "from cluster role",
			serverArg: provisioning.Server{
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{
										"192.168.0.100",
									},
									Roles: []string{
										"cluster",
									},
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			want:      "192.168.0.100:8443",
		},
		{
			name: "from management role fallback",
			serverArg: provisioning.Server{
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{
										"192.168.0.100",
									},
									Roles: []string{
										"management",
									},
								},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			want:      "192.168.0.100:8443",
		},
		{
			name: "without cluster and management role",
			serverArg: provisioning.Server{
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{
										"192.168.0.100",
									},
									Roles: []string{}, // management role missing
								},
							},
						},
					},
				},
			},

			assertErr: require.Error,
			want:      "",
		},
		{
			name: "cluster role set on interface without ip",
			serverArg: provisioning.Server{
				OSData: api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{}, // ip address missing
									Roles: []string{
										"cluster",
									},
								},
							},
						},
					},
				},
			},

			assertErr: require.Error,
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := determineClusterRoleAddress(tc.serverArg)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_clusterUpdateState(t *testing.T) {
	tests := []struct {
		name                          string
		clusterUpdateInProgressStatus api.ClusterUpdateInProgressStatus
		serverStates                  []api.ServerUpdateState

		want string
	}{
		{
			name: "error",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressError,
				Error:      "boom",
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpdatePending,
			},

			want: "boom",
		},
		{
			name: "inactive",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressInactive,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpdatePending,
			},

			want: "",
		},

		// Update only, without reboot.
		{
			name: "apply update without reboot - all servers pending",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdate,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[1/6] update pending server "serverA"`,
		},
		{
			name: "apply update without reboot - first server updating",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdate,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpdating,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[2/6] updating server "serverA"`,
		},
		{
			name: "apply update without reboot - first server updated, awaiting reboot",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdate,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[3/6] update pending server "serverB"`,
		},
		{
			name: "apply update without reboot - last server updating",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdate,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateUpdating,
			},

			want: `[6/6] updating server "serverC"`,
		},
		{
			name: "apply update without reboot - all servers updated",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdate,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
			},

			want: "",
		},
		{
			name: "apply update without reboot - all servers up to date",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdate,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: "",
		},

		// With reboot.
		{
			name: "apply update with reboot - all servers pending",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[ 1/27] update pending server "serverA"`,
		},
		{
			name: "apply update with reboot - first server updating",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpdating,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[ 2/27] updating server "serverA"`,
		},
		{
			name: "apply update with reboot - first server awaiting evacuation",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[ 3/27] update pending server "serverB"`,
		},
		{
			name: "apply update with reboot - all servers updated",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
			},

			want: `[ 7/27] evacuation pending server "serverA"`,
		},

		// Rolling restart.
		{
			name: "rolling restart - first server evacuating",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateEvacuating,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
			},

			want: `[ 8/27] evacuating server "serverA"`,
		},
		{
			name: "rolling restart - last server post restore",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateInMaintenancePostRestore,
			},

			want: `[27/27] post restore server "serverC"`,
		},
	}

	serverNames := []string{"serverA", "serverB", "serverC"}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			servers := make(provisioning.Servers, 0, len(tc.serverStates))
			for i, state := range tc.serverStates {
				servers = append(servers, clusterUpdateStateTestServer(t, serverNames[i], state))
			}

			got := clusterUpdateState(tc.clusterUpdateInProgressStatus, servers)

			require.Equal(t, tc.want, got)
		})
	}
}

func clusterUpdateStateTestServer(t *testing.T, name string, state api.ServerUpdateState) provisioning.Server {
	t.Helper()

	server := provisioning.Server{
		Name:         name,
		Cluster:      ptr.To("clusterA"),
		Type:         api.ServerTypeIncus,
		Status:       api.ServerStatusReady,
		StatusDetail: api.ServerStatusDetailNone,
		VersionData: api.ServerVersionData{
			Applications: []api.ApplicationVersionData{
				{
					Name: "incus",
				},
			},
			NeedsUpdate:   ptr.To(false),
			NeedsReboot:   ptr.To(false),
			InMaintenance: ptr.To(api.NotInMaintenance),
		},
	}

	switch state {
	case api.ServerUpdateStateUpToDate:

	case api.ServerUpdateStateUpdatePending:
		server.VersionData.NeedsUpdate = ptr.To(true)

	case api.ServerUpdateStateUpdating:
		server.StatusDetail = api.ServerStatusDetailReadyUpdatingOS

	case api.ServerUpdateStateEvacuationPending:
		server.VersionData.NeedsReboot = ptr.To(true)

	case api.ServerUpdateStateEvacuating:
		server.VersionData.NeedsReboot = ptr.To(true)
		server.VersionData.InMaintenance = ptr.To(api.InMaintenanceEvacuating)

	case api.ServerUpdateStateInMaintenanceRebootPending:
		server.VersionData.NeedsReboot = ptr.To(true)
		server.VersionData.InMaintenance = ptr.To(api.InMaintenanceEvacuated)

	case api.ServerUpdateStateInMaintenanceRebooting:
		server.Status = api.ServerStatusOffline
		server.StatusDetail = api.ServerStatusDetailOfflineRebooting
		server.VersionData.InMaintenance = ptr.To(api.InMaintenanceEvacuated)

	case api.ServerUpdateStateInMaintenanceRestorePending:
		server.VersionData.InMaintenance = ptr.To(api.InMaintenanceEvacuated)

	case api.ServerUpdateStateInMaintenanceRestoring:
		server.VersionData.InMaintenance = ptr.To(api.InMaintenanceRestoring)

	case api.ServerUpdateStateInMaintenancePostRestore:
		server.StatusDetail = api.ServerStatusDetailReadyRestoring

	default:
		t.Fatalf("unsupported server update state %q", state)
	}

	require.Equal(t, state, server.UpdateState())

	return server
}
