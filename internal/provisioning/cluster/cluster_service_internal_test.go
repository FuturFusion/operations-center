package cluster

import (
	"context"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	serviceMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
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

func Test_memberDependentAddress(t *testing.T) {
	serverWithInterfaces := func(name string, interfaces map[string]incusosapi.SystemNetworkInterfaceState) provisioning.Server {
		return provisioning.Server{
			Name: name,
			OSData: api.OSData{
				Network: incusosapi.SystemNetwork{
					State: incusosapi.SystemNetworkState{
						Interfaces: interfaces,
					},
				},
			},
		}
	}

	referenceServer := serverWithInterfaces("one", map[string]incusosapi.SystemNetworkInterfaceState{
		"eth0": {
			Addresses: []string{"10.0.0.1", "fd00::1"},
			Roles:     []string{"cluster"},
		},
		"eth1": {
			Addresses: []string{"10.1.0.1"},
			Roles:     []string{"storage"},
		},
	})

	targetServer := serverWithInterfaces("two", map[string]incusosapi.SystemNetworkInterfaceState{
		"eth0": {
			Addresses: []string{"10.0.0.2", "fd00::2"},
			Roles:     []string{"cluster"},
		},
		"eth1": {
			Addresses: []string{"10.1.0.2"},
			Roles:     []string{"storage"},
		},
	})

	tests := []struct {
		name                string
		referenceServerArg  provisioning.Server
		referenceAddressArg string
		targetServerArg     provisioning.Server

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                "empty address is not member dependent",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "",
			targetServerArg:     targetServer,

			want:      "",
			assertErr: require.NoError,
		},
		{
			name:                "IPv4 wildcard address is not member dependent",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "0.0.0.0",
			targetServerArg:     targetServer,

			want:      "0.0.0.0",
			assertErr: require.NoError,
		},
		{
			name:                "IPv6 wildcard address is not member dependent",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "::",
			targetServerArg:     targetServer,

			want:      "::",
			assertErr: require.NoError,
		},
		{
			name:                "hostname is not member dependent",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "linstor.local",
			targetServerArg:     targetServer,

			want:      "linstor.local",
			assertErr: require.NoError,
		},
		{
			name:                "IPv4 address of the cluster role",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "10.0.0.1",
			targetServerArg:     targetServer,

			want:      "10.0.0.2",
			assertErr: require.NoError,
		},
		{
			name:                "IPv6 address of the cluster role",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "fd00::1",
			targetServerArg:     targetServer,

			want:      "fd00::2",
			assertErr: require.NoError,
		},
		{
			name:                "address of the storage role",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "10.1.0.1",
			targetServerArg:     targetServer,

			want:      "10.1.0.2",
			assertErr: require.NoError,
		},
		{
			name:                "error - address not assigned to the reference server",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "10.2.0.1",
			targetServerArg:     targetServer,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to determine the role of the network interface of server "one" () with the address "10.2.0.1" assigned`)
			},
		},
		{
			name:                "error - target server without an address of the same IP family",
			referenceServerArg:  referenceServer,
			referenceAddressArg: "fd00::1",
			targetServerArg: serverWithInterfaces("two", map[string]incusosapi.SystemNetworkInterfaceState{
				"eth0": {
					Addresses: []string{"10.0.0.2"},
					Roles:     []string{"cluster"},
				},
			}),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Server "two" () does not have an address of the same IP family as "fd00::1" on a network interface with any of the roles [cluster]`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := memberDependentAddress(tc.referenceServerArg, tc.referenceAddressArg, tc.targetServerArg)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_memberDependentListenAddress(t *testing.T) {
	referenceServer := provisioning.Server{
		Name: "one",
		OSData: api.OSData{
			Network: incusosapi.SystemNetwork{
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: []string{"10.0.0.1"},
							Roles:     []string{"cluster"},
						},
					},
				},
			},
		},
	}

	targetServer := provisioning.Server{
		Name: "two",
		OSData: api.OSData{
			Network: incusosapi.SystemNetwork{
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: []string{"10.0.0.2"},
							Roles:     []string{"cluster"},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name                      string
		referenceListenAddressArg string

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                      "empty listen address is not member dependent",
			referenceListenAddressArg: "",

			want:      "",
			assertErr: require.NoError,
		},
		{
			name:                      "wildcard listen address is not member dependent",
			referenceListenAddressArg: "[::]:3366",

			want:      "[::]:3366",
			assertErr: require.NoError,
		},
		{
			name:                      "member dependent listen address keeps the port",
			referenceListenAddressArg: "10.0.0.1:3366",

			want:      "10.0.0.2:3366",
			assertErr: require.NoError,
		},
		{
			name:                      "error - listen address without port",
			referenceListenAddressArg: "10.0.0.1",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid listen address "10.0.0.1" of server "one"`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := memberDependentListenAddress(referenceServer, tc.referenceListenAddressArg, targetServer)

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
		newUpdateAvailable            bool

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

		// Servers, which have been evacuated before the update was triggered, are kept
		// evacuated and therefore have no restore step ahead of them.
		{
			name: "apply update with reboot - server evacuated before is not restored",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:      api.ClusterUpdateInProgressApplyUpdateWithReboot,
				EvacuatedBefore: []string{"serverA"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRestorePending,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
			},

			want: `[14/27] evacuation pending server "serverB"`,
		},
		{
			name: "rolling restart - server evacuated before is not restored",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:      api.ClusterUpdateInProgressRollingRestart,
				EvacuatedBefore: []string{"serverA"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRestorePending,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
			},

			// Identical to the apply update phase above, the phase transition must not
			// change the reported progress.
			want: `[14/27] evacuation pending server "serverB"`,
		},
		{
			name: "rolling restart - all servers done, server evacuated before stays evacuated",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:      api.ClusterUpdateInProgressRollingRestart,
				EvacuatedBefore: []string{"serverA"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRestorePending,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: "",
		},

		// States, which are not part of the rolling update, are counted as not started.
		{
			name: "rolling restart - undefined state counts as not started",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUndefined,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: `[19/27] undefined server "serverA"`,
		},
		{
			name: "rolling restart - rebooting outside of maintenance counts as not started",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateRebooting,
			},

			want: `[19/27] rebooting server "serverC"`,
		},
		{
			name: "apply update with reboot - server updating applications is reported as updating",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			},
			serverStates: []api.ServerUpdateState{
				serverUpdateStateUpdatingApplication,
				api.ServerUpdateStateUpdatePending,
				api.ServerUpdateStateUpdatePending,
			},

			want: `[ 2/27] updating server "serverA"`,
		},

		// A newly published update must not interrupt an ongoing rolling restart.
		{
			name: "rolling restart - newly available update does not rewind the progress",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRebootPending,
				api.ServerUpdateStateEvacuationPending,
				api.ServerUpdateStateEvacuationPending,
			},
			newUpdateAvailable: true,

			want: `[ 9/27] in maintenance, reboot pending server "serverA"`,
		},
		{
			name: "rolling reboot - all servers pending",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:    api.ClusterUpdateInProgressRollingReboot,
				PendingReboot: []string{"serverA", "serverB", "serverC"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: `[ 1/21] evacuation pending server "serverA"`,
		},
		{
			name: "rolling reboot - first server evacuated, awaiting its reboot",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:    api.ClusterUpdateInProgressRollingReboot,
				PendingReboot: []string{"serverA", "serverB", "serverC"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRestorePending,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: `[ 3/21] in maintenance, reboot pending server "serverA"`,
		},
		{
			name: "rolling reboot - first server rebooted, awaiting its restore",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:    api.ClusterUpdateInProgressRollingReboot,
				PendingReboot: []string{"serverB", "serverC"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRestorePending,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: `[ 5/21] in maintenance, restore pending server "serverA"`,
		},
		{
			name: "rolling reboot - server off the pending list is up to date",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:    api.ClusterUpdateInProgressRollingReboot,
				PendingReboot: []string{"serverB", "serverC"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: `[ 8/21] evacuation pending server "serverB"`,
		},
		{
			name: "rolling reboot - all servers done",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingReboot,
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},

			want: "",
		},
		{
			name: "rolling reboot - newly available update does not rewind the progress",
			clusterUpdateInProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress:    api.ClusterUpdateInProgressRollingReboot,
				PendingReboot: []string{"serverA", "serverB", "serverC"},
			},
			serverStates: []api.ServerUpdateState{
				api.ServerUpdateStateInMaintenanceRestorePending,
				api.ServerUpdateStateUpToDate,
				api.ServerUpdateStateUpToDate,
			},
			newUpdateAvailable: true,

			want: `[ 3/21] in maintenance, reboot pending server "serverA"`,
		},
	}

	serverNames := []string{"serverA", "serverB", "serverC"}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			servers := make(provisioning.Servers, 0, len(tc.serverStates))
			for i, state := range tc.serverStates {
				server := clusterUpdateStateTestServer(t, serverNames[i], state)
				if tc.newUpdateAvailable {
					server.VersionData.NeedsUpdate = ptr.To(true)
				}

				servers = append(servers, server)
			}

			got := clusterUpdateState(tc.clusterUpdateInProgressStatus, servers).String()

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_clusterUpdateState_phaseTransitionKeepsProgress(t *testing.T) {
	// The rolling update advances from the apply update phase to the rolling restart
	// phase once all servers are updated. Both phases have to report the same
	// progress for a given server state, otherwise the progress reported to the user
	// jumps at the moment the phase changes.
	serverNames := []string{"serverA", "serverB", "serverC"}

	// Only the restart states are relevant, the phase does not change before all
	// servers have passed the update states.
	for _, state := range clusterUpdateSteps[clusterUpdateUpdateSteps:] {
		t.Run(string(state), func(t *testing.T) {
			servers := make(provisioning.Servers, 0, len(serverNames))
			for _, name := range serverNames {
				servers = append(servers, clusterUpdateStateTestServer(t, name, state))
			}

			applyUpdate := clusterUpdateState(api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			}, servers)

			rollingRestart := clusterUpdateState(api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			}, servers)

			require.Equal(t, applyUpdate, rollingRestart)
		})
	}
}

func Test_clusterUpdateState_rollingRebootSkipsTheUpdateSteps(t *testing.T) {
	serverNames := []string{"serverA", "serverB", "serverC"}

	// The update steps are skipped altogether, so only the restart states can occur
	// during a rolling reboot.
	for _, state := range clusterUpdateSteps[clusterUpdateUpdateSteps:] {
		t.Run(string(state), func(t *testing.T) {
			servers := make(provisioning.Servers, 0, len(serverNames))
			for _, name := range serverNames {
				servers = append(servers, clusterUpdateStateTestServer(t, name, state))
			}

			rollingRestart := clusterUpdateState(api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingRestart,
			}, servers)

			rollingReboot := clusterUpdateState(api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressRollingReboot,
			}, servers)

			// The reboot reports the same server in the same state, but its scale is
			// shorter by the update steps of every server.
			skipped := clusterUpdateUpdateSteps * len(serverNames)

			require.Equal(t, rollingRestart.state, rollingReboot.state)
			require.Equal(t, rollingRestart.serverName, rollingReboot.serverName)
			require.Equal(t, rollingRestart.totalSteps-skipped, rollingReboot.totalSteps)
			require.Equal(t, rollingRestart.step-skipped, rollingReboot.step)
		})
	}
}

func Test_clusterUpdateState_doesNotReorderServers(t *testing.T) {
	// clusterUpdateState is called with the servers of a cluster while the caller is
	// iterating over them, so it must not reorder the slice it is given.
	servers := provisioning.Servers{
		clusterUpdateStateTestServer(t, "serverC", api.ServerUpdateStateUpdatePending),
		clusterUpdateStateTestServer(t, "serverA", api.ServerUpdateStateUpdatePending),
		clusterUpdateStateTestServer(t, "serverB", api.ServerUpdateStateUpdatePending),
	}

	got := clusterUpdateState(api.ClusterUpdateInProgressStatus{
		InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
	}, servers).String()

	require.Equal(t, `[ 1/27] update pending server "serverA"`, got)
	require.Equal(t, []string{"serverC", "serverA", "serverB"}, []string{servers[0].Name, servers[1].Name, servers[2].Name})
}

// serverUpdateStateUpdatingApplication is a test only marker for a server, which is
// updating its applications. api.Server.UpdateState reports "undefined" for it.
const serverUpdateStateUpdatingApplication = api.ServerUpdateState("updating application")

func clusterUpdateStateTestServer(t *testing.T, name string, state api.ServerUpdateState) provisioning.Server {
	t.Helper()

	wantState := state

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

	case api.ServerUpdateStateRebootPending:
		// A reboot is only pending outside of maintenance for servers, which are not
		// part of an Incus cluster.
		server.Cluster = nil
		server.VersionData.NeedsReboot = ptr.To(true)

	case api.ServerUpdateStateRebooting:
		server.Status = api.ServerStatusOffline
		server.StatusDetail = api.ServerStatusDetailOfflineRebooting

	case api.ServerUpdateStateUndefined:
		server.Status = api.ServerStatusUnknown

	case serverUpdateStateUpdatingApplication:
		server.StatusDetail = api.ServerStatusDetailReadyUpdatingApplication
		wantState = api.ServerUpdateStateUndefined

	default:
		t.Fatalf("unsupported server update state %q", state)
	}

	require.Equal(t, wantState, server.UpdateState())

	return server
}

func TestClusterService_getClusterUpdateStatus_progressOnlyMovesForward(t *testing.T) {
	// The update state of a server is derived from data, which is refreshed
	// asynchronously from several sources, so a server can briefly fall back to an
	// earlier state.
	serverStates := []api.ServerUpdateState{
		api.ServerUpdateStateUpdating,
		api.ServerUpdateStateUpdatePending, // Falls back, the version data is not refreshed yet.
		api.ServerUpdateStateEvacuationPending,
	}

	serverSvcGetAllWithFilter := make([]queue.Item[provisioning.Servers], 0, len(serverStates))
	for _, state := range serverStates {
		serverSvcGetAllWithFilter = append(serverSvcGetAllWithFilter, queue.Item[provisioning.Servers]{
			Value: provisioning.Servers{
				clusterUpdateStateTestServer(t, "serverA", state),
				clusterUpdateStateTestServer(t, "serverB", api.ServerUpdateStateUpdatePending),
				clusterUpdateStateTestServer(t, "serverC", api.ServerUpdateStateUpdatePending),
			},
		})
	}

	serverSvc := &serviceMock.ServerServiceMock{
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
			return queue.Pop(t, &serverSvcGetAllWithFilter)
		},
	}

	clusterSvc := New(nil, nil, nil, serverSvc, nil, nil, nil, nil)

	ctx := context.Background()

	got := make([]string, 0, len(serverStates))
	for range serverStates {
		clusterUpdateStatus := api.ClusterUpdateStatus{
			InProgressStatus: api.ClusterUpdateInProgressStatus{
				InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
			},
		}

		err := clusterSvc.getClusterUpdateStatus(ctx, "clusterA", &clusterUpdateStatus)
		require.NoError(t, err)

		got = append(got, ptr.From(clusterUpdateStatus.InProgressStatus.StatusDescription))
	}

	require.Equal(t, []string{
		`[ 2/27] updating server "serverA"`,
		`[ 2/27] updating server "serverA"`, // Held back, the calculated progress would be 1/27.
		`[ 3/27] update pending server "serverB"`,
	}, got)

	// Once the update is done, the recorded progress is dropped, so the next rolling
	// update of this cluster starts reporting from the beginning again.
	serverSvcGetAllWithFilter = []queue.Item[provisioning.Servers]{
		{
			Value: provisioning.Servers{
				clusterUpdateStateTestServer(t, "serverA", api.ServerUpdateStateUpToDate),
			},
		},
		{
			Value: provisioning.Servers{
				clusterUpdateStateTestServer(t, "serverA", api.ServerUpdateStateUpdatePending),
				clusterUpdateStateTestServer(t, "serverB", api.ServerUpdateStateUpdatePending),
				clusterUpdateStateTestServer(t, "serverC", api.ServerUpdateStateUpdatePending),
			},
		},
	}

	inactive := api.ClusterUpdateStatus{}
	err := clusterSvc.getClusterUpdateStatus(ctx, "clusterA", &inactive)
	require.NoError(t, err)
	require.Nil(t, inactive.InProgressStatus.StatusDescription)

	relaunched := api.ClusterUpdateStatus{
		InProgressStatus: api.ClusterUpdateInProgressStatus{
			InProgress: api.ClusterUpdateInProgressApplyUpdateWithReboot,
		},
	}

	err = clusterSvc.getClusterUpdateStatus(ctx, "clusterA", &relaunched)
	require.NoError(t, err)
	require.Equal(t, `[ 1/27] update pending server "serverA"`, ptr.From(relaunched.InProgressStatus.StatusDescription))
}

func TestClusterService_markServerRebooted(t *testing.T) {
	tests := []struct {
		name             string
		repoGetByNameErr error
		repoUpdateErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:          "error - repo.Update",
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	fixedTime := time.Date(2026, 3, 12, 8, 54, 35, 123, time.UTC)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ClusterRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Cluster, error) {
					if tc.repoGetByNameErr != nil {
						return nil, tc.repoGetByNameErr
					}

					return &provisioning.Cluster{
						Name: "clusterA",

						UpdateStatus: api.ClusterUpdateStatus{
							InProgressStatus: api.ClusterUpdateInProgressStatus{
								InProgress:    api.ClusterUpdateInProgressRollingReboot,
								PendingReboot: []string{"serverA", "serverB"},
							},
						},
					}, nil
				},
				UpdateFunc: func(ctx context.Context, cluster provisioning.Cluster) error {
					require.Equal(t, fixedTime, cluster.UpdateStatus.InProgressStatus.LastUpdated)
					require.Equal(t, []string{"serverB"}, cluster.UpdateStatus.InProgressStatus.PendingReboot)
					return tc.repoUpdateErr
				},
			}

			clusterSvc := New(
				repo, nil, nil, nil, nil, nil, nil, nil,
				WithNow(func() time.Time {
					return fixedTime
				}),
			)

			cluster := provisioning.Cluster{
				Name: "clusterA",

				UpdateStatus: api.ClusterUpdateStatus{
					InProgressStatus: api.ClusterUpdateInProgressStatus{
						InProgress: api.ClusterUpdateInProgressRollingReboot,
					},
				},
			}

			// Run test
			err := clusterSvc.markServerRebooted(context.Background(), cluster, "serverA")

			// Assert
			tc.assertErr(t, err)
		})
	}
}
