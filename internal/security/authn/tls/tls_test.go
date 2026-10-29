package tls_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	authntls "github.com/FuturFusion/operations-center/internal/security/authn/tls"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestTLS_Auth(t *testing.T) {
	t.Parallel()

	trustedCert := parseCertificate(t, testcert.ClientCertificate)
	untrustedCert := parseCertificate(t, testcert.SecondClientCertificate)

	tests := []struct {
		name     string
		tlsState *tls.ConnectionState

		assertErr    require.ErrorAssertionFunc
		wantTrusted  bool
		wantUsername string
		wantProtocol string
	}{
		{
			name: "success - trusted leaf certificate",
			tlsState: &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{trustedCert},
			},

			assertErr:    require.NoError,
			wantTrusted:  true,
			wantUsername: testcert.ClientCertificateFingerprint,
			wantProtocol: api.AuthenticationMethodTLS,
		},
		{
			name: "success - trusted leaf certificate followed by untrusted certificate",
			tlsState: &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{trustedCert, untrustedCert},
			},

			assertErr:    require.NoError,
			wantTrusted:  true,
			wantUsername: testcert.ClientCertificateFingerprint,
			wantProtocol: api.AuthenticationMethodTLS,
		},
		{
			name: "not trusted - untrusted leaf certificate",
			tlsState: &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{untrustedCert},
			},

			assertErr: require.NoError,
		},
		{
			name: "not trusted - untrusted leaf certificate followed by trusted certificate",
			tlsState: &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{untrustedCert, trustedCert},
			},

			assertErr: require.NoError,
		},
		{
			name:     "not trusted - no client certificate",
			tlsState: &tls.ConnectionState{},

			assertErr: require.NoError,
		},
		{
			name:     "error - no TLS",
			tlsState: nil,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			auther := authntls.New([]string{testcert.ClientCertificateFingerprint})

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			r.TLS = tc.tlsState

			// Run test
			trusted, username, protocol, err := auther.Auth(httptest.NewRecorder(), r)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantTrusted, trusted, "trusted")
			require.Equal(t, tc.wantUsername, username, "username")
			require.Equal(t, tc.wantProtocol, protocol, "protocol")
		})
	}
}

func parseCertificate(t *testing.T, certificatePEM string) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode([]byte(certificatePEM))
	require.NotNil(t, block, "certificate must be PEM encoded")

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	return cert
}
