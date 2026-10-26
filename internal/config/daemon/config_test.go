package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestUpdateNetwork(t *testing.T) {
	tests := []struct {
		name string
		cfg  system.NetworkPut

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name: "empty",
			cfg: system.NetworkPut{
				RestServerAddress: "",
			},

			assertErr: require.NoError,
		},
		{
			name: ":",
			cfg: system.NetworkPut{
				RestServerAddress: ":",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: ":1234",
			cfg: system.NetworkPut{
				RestServerAddress: ":1234",
			},

			assertErr: require.NoError,
			want:      "[::]:1234",
		},
		{
			name: "ipv4 - 0.0.0.0",
			cfg: system.NetworkPut{
				RestServerAddress: "0.0.0.0",
			},

			assertErr: require.NoError,
			want:      "0.0.0.0:7443",
		},
		{
			name: "ipv4 - 0.0.0.0:",
			cfg: system.NetworkPut{
				RestServerAddress: "0.0.0.0:",
			},

			assertErr: require.NoError,
			want:      "0.0.0.0:7443",
		},
		{
			name: "ipv6 - ::",
			cfg: system.NetworkPut{
				RestServerAddress: "::",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: "ipv6 - [::]",
			cfg: system.NetworkPut{
				RestServerAddress: "[::]",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: "ipv6 - [::]:",
			cfg: system.NetworkPut{
				RestServerAddress: "[::]:",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: "ipv6 - abcd::0",
			cfg: system.NetworkPut{
				RestServerAddress: "abcd::0",
			},

			assertErr: require.NoError,
			want:      "[abcd::]:7443",
		},

		// Errors
		{
			name: "error - :0",
			cfg: system.NetworkPut{
				RestServerAddress: ":0",
			},

			assertErr: require.Error,
			want:      "",
		},
		{
			name: "error - no ip",
			cfg: system.NetworkPut{
				RestServerAddress: "a.b.c.d",
			},

			assertErr: require.Error,
			want:      "",
		},
		{
			name: "error - no ip with port",
			cfg: system.NetworkPut{
				RestServerAddress: "a.b.c.d:7443",
			},

			assertErr: require.Error,
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config.InitTest(t, &mock.EnvironmentMock{
				ClusteredFunc: func() bool { return false },
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)

			err := config.UpdateLocalNetwork(t.Context(), tc.cfg)

			tc.assertErr(t, err)

			require.Equal(t, tc.want, config.GetLocalNetwork().RestServerAddress)
		})
	}
}
