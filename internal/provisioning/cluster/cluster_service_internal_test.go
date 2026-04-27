package cluster

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
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
