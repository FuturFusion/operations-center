package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func TestCluster_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cluster provisioning.Cluster

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			cluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			cluster: provisioning.Cluster{
				Name:            "", // invalid
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - server name list empty",
			cluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: nil, // invalid
				ConnectionURL:   "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL empty",
			cluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL invalid",
			cluster: provisioning.Cluster{
				Name:            "one",
				ServerHostnames: []string{"server1", "server2"},
				ConnectionURL:   ":|\\", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cluster.Validate()

			tc.assertErr(t, err)
		})
	}
}
