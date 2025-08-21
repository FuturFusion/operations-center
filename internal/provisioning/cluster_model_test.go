package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
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
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			cluster: provisioning.Cluster{
				Name:          "", // invalid
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - name prohibited character",
			cluster: provisioning.Cluster{
				Name:          "foo/bar", // "/" is prohibited
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - server name list empty",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   nil, // invalid
				ConnectionURL: "http://one/",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - connection URL invalid",
			cluster: provisioning.Cluster{
				Name:          "one",
				ServerNames:   []string{"server1", "server2"},
				ConnectionURL: ":|\\", // invalid
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

func TestCluster_Filter(t *testing.T) {
	tests := []struct {
		name   string
		filter provisioning.ClusterFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: provisioning.ClusterFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: provisioning.ClusterFilter{
				Expression: ptr.To("true"),
			},

			want: `filter=true`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.String())
		})
	}
}
