package client_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestNew_serverCertificateVerification(t *testing.T) {
	serverCert, serverX509Cert := generateServerCert(t)
	_, otherX509Cert := generateServerCert(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{"auth":"untrusted"}}`))
	}))

	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	srv.StartTLS()
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		opts []client.Option

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - certificate of the server is pinned",
			opts: []client.Option{client.WithTrustedServerCertificate(serverX509Cert)},

			assertErr: require.NoError,
		},
		{
			name: "error - no certificate pinned, self-signed certificate is not trusted",
			opts: nil,

			assertErr: require.Error,
		},
		{
			name: "error - pinned certificate does not match the one presented by the server",
			opts: []client.Option{client.WithTrustedServerCertificate(otherX509Cert)},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := client.New(srv.URL, tc.opts...)
			require.NoError(t, err)

			_, err = c.DoRequest(t.Context(), http.MethodGet, "", url.Values{}, nil)
			tc.assertErr(t, err)
		})
	}
}

func TestIsServerTrusted(t *testing.T) {
	serverCert, serverX509Cert := generateServerCert(t)
	_, otherX509Cert := generateServerCert(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{"auth":"untrusted"}}`))
	}))

	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	srv.StartTLS()
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		opts       []client.Option
		pinnedCert api.Certificate

		wantTrusted    bool
		wantActualCert *x509.Certificate
	}{
		{
			name:        "trusted - certificate of the server is pinned",
			opts:        []client.Option{client.WithTrustedServerCertificate(serverX509Cert)},
			pinnedCert:  api.Certificate{Certificate: serverX509Cert},
			wantTrusted: true,
		},
		{
			name: "untrusted - no certificate pinned, self-signed certificate is not trusted",
			opts: nil,

			wantTrusted:    false,
			wantActualCert: serverX509Cert,
		},
		{
			name:       "untrusted - pinned certificate does not match the one presented by the server",
			opts:       []client.Option{client.WithTrustedServerCertificate(otherX509Cert)},
			pinnedCert: api.Certificate{Certificate: otherX509Cert},

			wantTrusted:    false,
			wantActualCert: serverX509Cert,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := client.New(srv.URL, tc.opts...)
			require.NoError(t, err)

			actualCert, trusted, err := c.IsServerTrusted(t.Context(), tc.pinnedCert)
			require.NoError(t, err)
			require.Equal(t, tc.wantTrusted, trusted)

			if tc.wantActualCert != nil {
				require.NotNil(t, actualCert.Certificate)
				require.True(t, tc.wantActualCert.Equal(actualCert.Certificate))
			} else {
				require.Nil(t, actualCert.Certificate)
			}
		})
	}
}

func TestWithTrustedCertificate_doesNotModifyProvidedCertificate(t *testing.T) {
	_, serverX509Cert := generateServerCert(t)

	require.False(t, serverX509Cert.IsCA)

	_, err := client.New("https://localhost:8443", client.WithTrustedServerCertificate(serverX509Cert))
	require.NoError(t, err)

	// The certificate provided by the caller is not turned into a CA, only the
	// copy held by the TLS config is.
	require.False(t, serverX509Cert.IsCA)
}

func generateServerCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, true)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	return cert, x509Cert
}
