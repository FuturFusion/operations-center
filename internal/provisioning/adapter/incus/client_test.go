package incus_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v6/shared/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/incus"
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
		response   []byte
		setup      func(*httptest.Server)

		assertErr require.ErrorAssertionFunc
		wantPath  string
	}{
		{
			name:       "success",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusOK,
			response: []byte(`{
  "metadata": {}
}`),
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPath:  "/1.0",
		},
		{
			name:    "error - invalid key pair",
			certPEM: certPEM,
			keyPEM:  certPEM, // invalid, should be key
			setup:   func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
		{
			name:    "error - connection failure",
			certPEM: certPEM,
			keyPEM:  keyPEM,
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
			wantPath:  "/1.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPath string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
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

			client := incus.New(tc.certPEM, tc.keyPEM)

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
			require.Equal(t, tc.wantPath, gotPath)
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
		wantPath      string
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
			wantPath:  "/1.0/resources",
			wantResources: api.HardwareData{
				Resources: incusapi.Resources{
					CPU: incusapi.ResourcesCPU{
						Architecture: "x86_64",
					},
				},
			},
		},
		{
			name:    "error - invalid key pair",
			certPEM: certPEM,
			keyPEM:  certPEM, // invalid, should be key
			setup:   func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
		{
			name:    "error - connection failure",
			certPEM: certPEM,
			keyPEM:  keyPEM,
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
			wantPath:  "/1.0/resources",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPath string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
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

			client := incus.New(tc.certPEM, tc.keyPEM)

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
			require.Equal(t, tc.wantPath, gotPath)
			require.Equal(t, tc.wantResources, resources)
		})
	}
}

func TestClient_GetOSData(t *testing.T) {
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
		wantPath      string
		wantResources api.OSData
	}{
		{
			name:       "success",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusOK,
			response: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPath:  "/os/1.0/system/network",
			wantResources: api.OSData{
				Network: incusosapi.SystemNetwork{
					Config: &incusosapi.SystemNetworkConfig{
						DNS: &incusosapi.SystemNetworkDNS{
							Hostname: "foobar",
							Domain:   "local",
						},
					},
				},
			},
		},
		{
			name:    "error - invalid key pair",
			certPEM: certPEM,
			keyPEM:  certPEM, // invalid, should be key
			setup:   func(_ *httptest.Server) {},

			assertErr: require.Error,
		},
		{
			name:    "error - connection failure",
			certPEM: certPEM,
			keyPEM:  keyPEM,
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
			wantPath:  "/os/1.0/system/network",
		},
		{
			name:       "error - invalid JSON",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusInternalServerError,
			response: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPath:  "/os/1.0/system/network",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPath string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
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

			client := incus.New(tc.certPEM, tc.keyPEM)

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
			resources, err := client.GetOSData(ctx, target)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPath, gotPath)
			require.Equal(t, tc.wantResources, resources)
		})
	}
}
