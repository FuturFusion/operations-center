package provisioning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_clusterEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint ClusterEndpoint

		wantConnectionURL      string
		wantCertificate        string
		assertGetServerNameErr require.ErrorAssertionFunc
		wantServerName         string
	}{
		{
			name:     "empty",
			endpoint: ClusterEndpoint{},

			assertGetServerNameErr: require.Error,
		},
		{
			name: "one",
			endpoint: ClusterEndpoint{
				{
					ConnectionURL: "http://one/",
					Certificate:   "cert",
				},
			},

			wantConnectionURL:      "http://one/",
			wantCertificate:        "cert",
			assertGetServerNameErr: require.NoError,
			wantServerName:         "one",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCertificate := tc.endpoint.GetCertificate()
			gotConnectionURL := tc.endpoint.GetConnectionURL()
			gotServerName, err := tc.endpoint.GetServerName()
			tc.assertGetServerNameErr(t, err)

			require.Equal(t, tc.wantCertificate, gotCertificate)
			require.Equal(t, tc.wantConnectionURL, gotConnectionURL)
			require.Equal(t, tc.wantServerName, gotServerName)
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
