package provisioning

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"
)

func Test_detectInterface(t *testing.T) {
	tests := []struct {
		name    string
		network incusosapi.SystemNetwork

		wantNic string
	}{
		{
			name: "default - empty system network state",

			wantNic: "enp5s0",
		},
		{
			name: "interface with clustering role and IP address",
			network: incusosapi.SystemNetwork{
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: []string{"192.168.1.2"},
							Roles:     []string{"clustering"},
						},
						"eth1": {
							Roles: []string{"clustering"},
						},
						"eth2": {
							Addresses: []string{"192.168.1.2"},
						},
					},
				},
			},

			wantNic: "eth0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nic := detectClusteringInterface(tc.network)

			require.Equal(t, tc.wantNic, nic)
		})
	}
}
