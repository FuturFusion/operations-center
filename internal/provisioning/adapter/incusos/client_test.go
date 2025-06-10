package incusos_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	incusapi "github.com/lxc/incus/v6/shared/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/incusos"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClient_Ping(t *testing.T) {
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	certPEM, keyPEM := string(certPEMByte), string(keyPEMByte)

	tests := []struct {
		name       string
		certPEM    string
		keyPEM     string
		statusCode int
		setup      func(*httptest.Server)

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "success",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusOK,
			setup:      func(_ *httptest.Server) {},

			assertErr: require.NoError,
		},
		{
			name:       "error - invalid key pair",
			certPEM:    certPEM,
			keyPEM:     certPEM, // invalid, should be key
			statusCode: http.StatusOK,
			setup:      func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
		{
			name:       "error - connection failure",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusInternalServerError,
			setup: func(server *httptest.Server) {
				server.Close()
			},

			assertErr: require.Error,
		},
		{
			name:       "error - unexpected http status code",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusInternalServerError,
			setup:      func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			server.TLS = &tls.Config{
				NextProtos: []string{"h2", "http/1.1"},
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  caPool,
			}

			server.StartTLS()
			defer server.Close()

			tc.setup(server)

			client := incusos.New(tc.certPEM, tc.keyPEM)

			ctx := context.Background()

			serverCert := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: server.Certificate().Raw,
			})

			target := provisioning.Server{
				ConnectionURL: server.URL,
				Certificate:   string(serverCert),
			}

			// Run test
			err = client.Ping(ctx, target)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClient_GetResources(t *testing.T) {
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	certPEM, keyPEM := string(certPEMByte), string(keyPEMByte)

	tests := []struct {
		name       string
		certPEM    string
		keyPEM     string
		statusCode int
		response   []byte
		setup      func(*httptest.Server)

		assertErr     require.ErrorAssertionFunc
		wantResources api.HardwareData
	}{
		{
			name:       "success",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusOK,
			response: []byte(`{
  "metadata": {
    "cpu": {
      "architecture": "x86_64"
    }
  }
}`),
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantResources: api.HardwareData{
				Resources: incusapi.Resources{
					CPU: incusapi.ResourcesCPU{
						Architecture: "x86_64",
					},
				},
			},
		},
		{
			name:       "error - invalid key pair",
			certPEM:    certPEM,
			keyPEM:     certPEM, // invalid, should be key
			statusCode: http.StatusOK,
			setup:      func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
		{
			name:       "error - connection failure",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusInternalServerError,
			setup: func(server *httptest.Server) {
				server.Close()
			},

			assertErr: require.Error,
		},
		{
			name:       "error - unexpected http status code",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusInternalServerError,
			setup:      func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
		{
			name:       "error - unexpected http status code",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusOK,
			response:   []byte(`{`), // invalid JSON
			setup:      func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write(tc.response)
			}))
			server.TLS = &tls.Config{
				NextProtos: []string{"h2", "http/1.1"},
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  caPool,
			}

			server.StartTLS()
			defer server.Close()

			tc.setup(server)

			client := incusos.New(tc.certPEM, tc.keyPEM)

			ctx := context.Background()

			serverCert := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: server.Certificate().Raw,
			})

			target := provisioning.Server{
				ConnectionURL: server.URL,
				Certificate:   string(serverCert),
			}

			// Run test
			resources, err := client.GetResources(ctx, target)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantResources, resources)
		})
	}
}
