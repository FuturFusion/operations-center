package provisioning

import (
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"
)

func Test_detectPrimaryNic(t *testing.T) {
	tests := []struct {
		name    string
		network incusosapi.SystemNetwork

		wantNic string
	}{
		{
			name: "default - empty system network",

			wantNic: "enp5s0",
		},
		{
			name: "default - empty system network config",
			network: incusosapi.SystemNetwork{
				Config: &incusosapi.SystemNetworkConfig{},
			},

			wantNic: "enp5s0",
		},
		{
			name: "interface with primary role",
			network: incusosapi.SystemNetwork{
				Config: &incusosapi.SystemNetworkConfig{
					Interfaces: []incusosapi.SystemNetworkInterface{
						{
							Name:  "eth0",
							Roles: []string{},
						},
						{
							Name:  "eth1",
							Roles: []string{"primary"},
						},
					},
				},
			},

			wantNic: "eth1",
		},
		{
			name: "interface with ip address",
			network: incusosapi.SystemNetwork{
				Config: &incusosapi.SystemNetworkConfig{
					Interfaces: []incusosapi.SystemNetworkInterface{
						{
							Name: "eth0",
						},
						{
							Name: "eth1",
						},
					},
				},
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: []string{},
						},
						"eth1": {
							Addresses: []string{
								"192.168.1.2",
							},
						},
					},
				},
			},

			wantNic: "eth1",
		},
		{
			name: "fallback to first interface",
			network: incusosapi.SystemNetwork{
				Config: &incusosapi.SystemNetworkConfig{
					Interfaces: []incusosapi.SystemNetworkInterface{
						{
							Name: "eth0",
						},
						{
							Name: "eth1",
						},
					},
				},
				State: incusosapi.SystemNetworkState{
					Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
						"eth0": {
							Addresses: []string{},
						},
					},
				},
			},

			wantNic: "eth0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nic := detectPrimaryNic(tc.network)

			require.Equal(t, tc.wantNic, nic)
		})
	}
}
