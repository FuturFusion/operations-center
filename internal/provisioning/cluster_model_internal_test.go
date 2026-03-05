package provisioning

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func Test_clusterEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint ClusterEndpoint

		wantConnectionURL      string
		wantCertificate        string
		assertGetServerNameErr require.ErrorAssertionFunc
		wantServerName         string
		wantName               string
	}{
		{
			name:     "empty",
			endpoint: ClusterEndpoint{},

			assertGetServerNameErr: require.Error,
		},
		{
			name: "one - no cluster",
			endpoint: ClusterEndpoint{
				{
					Name:          "server",
					ConnectionURL: "http://one/",
					Certificate:   "cert",
				},
			},

			wantConnectionURL:      "http://one/",
			wantCertificate:        "cert",
			assertGetServerNameErr: require.NoError,
			wantServerName:         "one",
			wantName:               "server",
		},
		{
			name: "one - with cluster",
			endpoint: ClusterEndpoint{
				{
					Name:               "server",
					Cluster:            ptr.To("cluster"),
					ConnectionURL:      "http://one/",
					Certificate:        "cert",
					ClusterCertificate: ptr.To("cluster cert"),
				},
			},

			wantConnectionURL:      "http://one/",
			wantCertificate:        "cluster cert",
			assertGetServerNameErr: require.NoError,
			wantServerName:         "one",
			wantName:               "cluster",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCertificate := tc.endpoint.GetCertificate()
			gotConnectionURL := tc.endpoint.GetConnectionURL()
			gotServerName, err := tc.endpoint.GetServerName()
			gotName := tc.endpoint.GetName()
			tc.assertGetServerNameErr(t, err)

			require.Equal(t, tc.wantCertificate, gotCertificate)
			require.Equal(t, tc.wantConnectionURL, gotConnectionURL)
			require.Equal(t, tc.wantServerName, gotServerName)
			require.Equal(t, tc.wantName, gotName)
		})
	}
}

func Test_clusterEndpoint_many(t *testing.T) {
	endpoint := ClusterEndpoint{
		{
			ConnectionURL: "http://one/",
			Certificate:   "cert",
		},
		{
			ConnectionURL: "http://two/",
			Certificate:   "cert2",
		},
	}

	for endpoint := range endpoint.GetEndpoints() {
		require.Equal(t, "cert", endpoint.GetCertificate())
		break
	}
}
