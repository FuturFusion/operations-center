package flasher_test

import (
	"crypto/tls"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/flasher"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestFlasher_GetProviderConfig(t *testing.T) {
	const serverURL = "https://localhost"
	tokenID := uuidgen.FromPattern(t, "1")

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	tests := []struct {
		name      string
		serverURL string

		assertErr require.ErrorAssertionFunc
		want      *api.TokenProviderConfig
	}{
		{
			name:      "success",
			serverURL: serverURL,

			assertErr: require.NoError,
			want: &api.TokenProviderConfig{
				SystemProviderConfig: incusosapi.SystemProviderConfig{
					Name: "operations-center",
					Config: map[string]string{
						"server_url":         serverURL,
						"server_token":       tokenID.String(),
						"server_certificate": string(certPEM),
					},
				},
				Version: "1",
			},
		},
		{
			name:      "success",
			serverURL: "",

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			flasher := flasher.New(tc.serverURL, cert)

			// Run test
			got, err := flasher.GetProviderConfig(t.Context(), tokenID)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
