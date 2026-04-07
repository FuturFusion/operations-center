package provisioning

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_determineManagementAddress(t *testing.T) {
	tests := []struct {
		name      string
		serverArg Server

		want string
	}{
		{
			name: "from management role",
			serverArg: Server{
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
			serverArg: Server{
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
		serverArg Server

		want string
	}{
		{
			name: "from cluster role",
			serverArg: Server{
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
										"cluster",
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
			name: "from management role fallback",
			serverArg: Server{
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
			name: "without cluster and management role",
			serverArg: Server{
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

			want: "10.10.10.10:8443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := determineClusterRoleAddress(tc.serverArg)

			require.Equal(t, tc.want, got)
		})
	}
}
