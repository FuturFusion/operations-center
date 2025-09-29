package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateNetwork(t *testing.T) {
	tests := []struct {
		name string
		cfg  api.SystemNetworkPut

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name: "empty",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "",
				OperationsCenterAddress: "",
			},

			assertErr: require.NoError,
		},
		{
			name: ":",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       ":",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: ":1234",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       ":1234",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "[::]:1234",
		},
		{
			name: "ipv4 - 0.0.0.0",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "0.0.0.0",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "0.0.0.0:7443",
		},
		{
			name: "ipv4 - 0.0.0.0:",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "0.0.0.0:",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "0.0.0.0:7443",
		},
		{
			name: "ipv6 - ::",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "::",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: "ipv6 - [::]",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "[::]",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: "ipv6 - [::]:",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "[::]:",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "[::]:7443",
		},
		{
			name: "ipv6 - abcd::0",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "abcd::0",
				OperationsCenterAddress: "https://localhost:7443",
			},

			assertErr: require.NoError,
			want:      "[abcd::]:7443",
		},

		// Errors
		{
			name: "error - OperationsCenterAddress not set",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "127.0.0.1:7443",
				OperationsCenterAddress: "",
			},

			assertErr: require.Error,
			want:      "",
		},
		{
			name: "error - :0",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       ":0",
				OperationsCenterAddress: "",
			},

			assertErr: require.Error,
			want:      "",
		},
		{
			name: "error - no ip",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "a.b.c.d",
				OperationsCenterAddress: "",
			},

			assertErr: require.Error,
			want:      "",
		},
		{
			name: "error - no ip with port",
			cfg: api.SystemNetworkPut{
				RestServerAddress:       "a.b.c.d:7443",
				OperationsCenterAddress: "",
			},

			assertErr: require.Error,
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config.InitTest(t, &mock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			})

			err := config.UpdateNetwork(t.Context(), tc.cfg)

			tc.assertErr(t, err)

			require.Equal(t, tc.want, config.GetNetwork().RestServerAddress)
		})
	}
}
