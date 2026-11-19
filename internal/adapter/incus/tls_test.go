package incus_test

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func Test(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			t.Cleanup(server.Close)

			c := incus.New("", "", incus.WithSkipGetServer(true))
			cert, err := c.GetRemoteCertificate(t.Context(), provisioning.Server{
				ConnectionURL: server.URL,
			})
			require.NoError(t, err)

			require.NotEmpty(t, cert.Raw)
		})
	}
}

func TestClient_GetPublicCertificate(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	tlsServer := httptest.NewTLSServer(handler)
	t.Cleanup(tlsServer.Close)

	plainServer := httptest.NewServer(handler)
	t.Cleanup(plainServer.Close)

	tests := []struct {
		name       string
		publicURL  string
		httpClient *http.Client

		assertErr  require.ErrorAssertionFunc
		assertCert func(t *testing.T, cert *x509.Certificate)
	}{
		{
			name:       "success",
			publicURL:  tlsServer.URL,
			httpClient: tlsServer.Client(),

			assertErr: require.NoError,
			assertCert: func(t *testing.T, cert *x509.Certificate) {
				t.Helper()
				require.Equal(t, tlsServer.Certificate().Raw, cert.Raw)
			},
		},
		{
			name:       "error - certificate not trusted",
			publicURL:  tlsServer.URL,
			httpClient: &http.Client{},

			assertErr: require.Error,
			assertCert: func(t *testing.T, cert *x509.Certificate) {
				t.Helper()
				require.Nil(t, cert)
			},
		},
		{
			name:       "error - no TLS",
			publicURL:  plainServer.URL,
			httpClient: &http.Client{},

			assertErr: require.Error,
			assertCert: func(t *testing.T, cert *x509.Certificate) {
				t.Helper()
				require.Nil(t, cert)
			},
		},
		{
			name:       "error - invalid URL",
			publicURL:  ":|\\",
			httpClient: &http.Client{},

			assertErr: require.Error,
			assertCert: func(t *testing.T, cert *x509.Certificate) {
				t.Helper()
				require.Nil(t, cert)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			c := incus.New("", "", incus.WithHTTPClient(tc.httpClient))

			// Run test
			cert, err := c.GetPublicCertificate(t.Context(), tc.publicURL)

			// Assert
			tc.assertErr(t, err)

			tc.assertCert(t, cert)
		})
	}
}
