package terraform

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"
)

func Test_detectClusterInterface(t *testing.T) {
	tests := []struct {
		name    string
		network incusosapi.SystemNetwork

		wantNic string
	}{
		{
			name: "default - empty system network state",

			wantNic: "",
		},
		{
			name: "interface with clustering role and IP address",
			network: incusosapi.SystemNetwork{
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: []string{"192.168.1.2"},
							Roles:     []string{"cluster"},
						},
						"eth1": {
							Roles: []string{"cluster"},
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
			nic := detectClusterInterface(tc.network)

			require.Equal(t, tc.wantNic, nic)
		})
	}
}
