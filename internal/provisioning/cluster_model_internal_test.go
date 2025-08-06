package provisioning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_clusterEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint ClusterEndpoint

		wantConnectionURL string
		wantCertificate   string
	}{
		{
			name:     "empty",
			endpoint: ClusterEndpoint{},
		},
		{
			name: "one",
			endpoint: ClusterEndpoint{
				{
					ConnectionURL: "http://one/",
					Certificate:   "cert",
				},
			},

			wantConnectionURL: "http://one/",
			wantCertificate:   "cert",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCertificate := tc.endpoint.GetCertificate()
			gotConnectionURL := tc.endpoint.GetConnectionURL()

			require.Equal(t, tc.wantCertificate, gotCertificate)
			require.Equal(t, tc.wantConnectionURL, gotConnectionURL)
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
