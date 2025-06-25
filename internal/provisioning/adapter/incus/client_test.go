package incus_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v6/shared/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
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
		name     string
		certPEM  string
		keyPEM   string
		response []queue.Item[struct {
			statusCode   int
			responseBody []byte
		}]
		setup func(*httptest.Server)

		assertErr     require.ErrorAssertionFunc
		wantPaths     []string
		wantResources api.OSData
	}{
		{
			name:    "success",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[struct {
				statusCode   int
				responseBody []byte
			}]{
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
					},
				},
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "config": {
      "recovery_keys": [ "very secret recovery key" ]
    }
  }
}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPaths: []string{"/os/1.0/system/network", "/os/1.0/system/encryption"},
			wantResources: api.OSData{
				Network: incusosapi.SystemNetwork{
					Config: &incusosapi.SystemNetworkConfig{
						DNS: &incusosapi.SystemNetworkDNS{
							Hostname: "foobar",
							Domain:   "local",
						},
					},
				},
				Encryption: incusosapi.SystemEncryption{
					Config: struct {
						RecoveryKeys []string `json:"recovery_keys" yaml:"recovery_keys"`
					}{
						RecoveryKeys: []string{"very secret recovery key"},
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
			name:    "error - network data unexpected http status code",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[struct {
				statusCode   int
				responseBody []byte
			}]{
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
					},
				},
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusInternalServerError,
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPaths: []string{"/os/1.0/system/network", "/os/1.0/system/encryption"},
		},
		{
			name:    "error - network data invalid JSON",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[struct {
				statusCode   int
				responseBody []byte
			}]{
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "config": {
      "dns": {
        "hostname": "foobar",
        "domain": "local"
      }
    }
  }
}`),
					},
				},
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPaths: []string{"/os/1.0/system/network", "/os/1.0/system/encryption"},
		},
		{
			name:    "error - encryption data unexpected http status code",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[struct {
				statusCode   int
				responseBody []byte
			}]{
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusInternalServerError,
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPaths: []string{"/os/1.0/system/network"},
		},
		{
			name:    "error - encryption data invalid JSON",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[struct {
				statusCode   int
				responseBody []byte
			}]{
				{
					Value: struct {
						statusCode   int
						responseBody []byte
					}{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPaths: []string{"/os/1.0/system/network"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPaths []string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				response, _ := queue.Pop(t, &tc.response)
				w.WriteHeader(response.statusCode)
				_, _ = w.Write(response.responseBody)
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
			require.Equal(t, tc.wantPaths, gotPaths)
			require.Equal(t, tc.wantResources, resources)
			require.Empty(t, tc.response)
		})
	}
}

func TestClient_EnableOSServiceLVM(t *testing.T) {
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
			wantPath:  "/os/1.0/services/lvm",
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
			wantPath:  "/os/1.0/services/lvm",
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
			err = client.EnableOSServiceLVM(ctx, target)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPath, gotPath)
		})
	}
}

func TestClient_SetServerConfig(t *testing.T) {
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	certPEM, keyPEM := string(certPEMByte), string(keyPEMByte)

	type response struct {
		statusCode   int
		responseBody []byte
	}

	tests := []struct {
		name     string
		certPEM  string
		keyPEM   string
		response []queue.Item[response]
		setup    func(*httptest.Server)

		assertErr      require.ErrorAssertionFunc
		wantPath       string
		assertResponse func(tt require.TestingT, responseBody string, serverAddress string)
	}{
		{
			name:    "success",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {}
}`),
					},
				},
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {}
}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPath:  "/1.0",
			assertResponse: func(tt require.TestingT, responseBody string, serverAddress string) {
				serverAddressURL, _ := url.Parse(serverAddress)
				require.Contains(tt, responseBody, `"cluster.https_address":"`+serverAddressURL.Host+`"`)
			},
		},
		{
			name:    "error - invalid key pair",
			certPEM: certPEM,
			keyPEM:  certPEM, // invalid, should be key
			setup:   func(_ *httptest.Server) {},

			assertErr:      require.Error,
			assertResponse: func(tt require.TestingT, responseBody string, serverAddress string) {},
		},
		{
			name:    "error - connection failure",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			setup: func(server *httptest.Server) {
				server.Close()
			},

			assertErr:      require.Error,
			assertResponse: func(tt require.TestingT, responseBody string, serverAddress string) {},
		},
		{
			name:    "error - GetServer - unexpected http status code",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				{
					Value: response{
						statusCode: http.StatusInternalServerError,
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr:      require.Error,
			wantPath:       "/1.0",
			assertResponse: func(tt require.TestingT, responseBody string, serverAddress string) {},
		},
		{
			name:    "error - UpdateServer - unexpected http status code",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {}
}`),
					},
				},
				{
					Value: response{
						statusCode: http.StatusInternalServerError,
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr:      require.Error,
			wantPath:       "/1.0",
			assertResponse: func(tt require.TestingT, responseBody string, serverAddress string) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPath string
			var gotRequest string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path

				// We only care about the second request to UpdateServer
				body, _ := io.ReadAll(r.Body)
				gotRequest = string(body)

				response, _ := queue.Pop(t, &tc.response)
				w.WriteHeader(response.statusCode)
				_, _ = w.Write(response.responseBody)
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
			serverAddressURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			err = client.SetServerConfig(ctx, target, map[string]string{
				"cluster.https_address": serverAddressURL.Host,
			})

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPath, gotPath)
			tc.assertResponse(t, gotRequest, server.URL)
			require.Empty(t, tc.response)
		})
	}
}

func TestClient_EnableCluster(t *testing.T) {
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	certPEM, keyPEM := string(certPEMByte), string(keyPEMByte)

	type response struct {
		statusCode   int
		responseBody []byte
	}

	tests := []struct {
		name            string
		certPEM         string
		keyPEM          string
		response        []queue.Item[response]
		setup           func(*httptest.Server)
		assertErr       require.ErrorAssertionFunc
		wantCertificate string
		wantPaths       []string
	}{
		{
			name:    "success",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {}
}`),
					},
				},
				// /1.0/operations//wait
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "metadata":{
      "certificate": "certificate"
    }
  }
}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr:       require.NoError,
			wantCertificate: "certificate",
			wantPaths:       []string{"/1.0/events", "/1.0/cluster", "/1.0/operations//wait"},
		},
		{
			name:    "success - no certificate returned",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {}
}`),
					},
				},
				// /1.0/operations//wait
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "metadata":{
    }
  }
}`), // no certificate returned
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr:       require.NoError,
			wantCertificate: "",
			wantPaths:       []string{"/1.0/events", "/1.0/cluster", "/1.0/operations//wait"},
		},
		{
			name:    "success - no certificate returned",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {}
}`),
					},
				},
				// /1.0/operations//wait
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "metadata":{
      "certificate": {}
    }
  }
}`), // invalid type for certificate
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr:       require.NoError,
			wantCertificate: "",
			wantPaths:       []string{"/1.0/events", "/1.0/cluster", "/1.0/operations//wait"},
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
			name:    "error - fail op.WaitContext",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster
				{
					Value: response{
						statusCode:   http.StatusOK,
						responseBody: []byte(`{"metadata":{}}`),
					},
				},
				// /1.0/operations//wait
				{
					Value: response{
						statusCode:   http.StatusInternalServerError, // fail op.WaitContext
						responseBody: []byte(`{}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPaths: []string{"/1.0/events", "/1.0/cluster", "/1.0/operations//wait"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPaths []string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				response, _ := queue.Pop(t, &tc.response)
				w.WriteHeader(response.statusCode)
				_, _ = w.Write(response.responseBody)
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
			certificate, err := client.EnableCluster(ctx, target)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantCertificate, certificate)
			require.Equal(t, tc.wantPaths, gotPaths)
			require.Empty(t, tc.response)
		})
	}
}

func TestClient_GetClusterNodeNames(t *testing.T) {
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

		assertErr      require.ErrorAssertionFunc
		wantPath       string
		nodeNamesCount int
	}{
		{
			name:       "success",
			certPEM:    certPEM,
			keyPEM:     keyPEM,
			statusCode: http.StatusOK,
			response: []byte(`{
  "metadata": [ "https://127.0.0.1/cluster/members/one" ]
}`),
			setup: func(_ *httptest.Server) {},

			assertErr:      require.NoError,
			wantPath:       "/1.0/cluster/members",
			nodeNamesCount: 1,
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
			nodeNames, err := client.GetClusterNodeNames(ctx, target)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPath, gotPath)
			require.Len(t, nodeNames, tc.nodeNamesCount)
		})
	}
}

func TestClient_GetClusterJoinToken(t *testing.T) {
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	certPEM, keyPEM := string(certPEMByte), string(keyPEMByte)

	type response struct {
		statusCode   int
		responseBody []byte
	}

	tests := []struct {
		name      string
		certPEM   string
		keyPEM    string
		response  []queue.Item[response]
		setup     func(*httptest.Server)
		assertErr require.ErrorAssertionFunc
		wantPaths []string
		wantToken string
	}{
		{
			name:    "success",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster/members
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "metadata": {
      "serverName": "server1",
      "secret": "secret",
      "fingerprint": "fingerprint",
      "addresses": ["1.0.0.1", "1.0.0.2"],
      "expiresAt": "2025-06-17T15:39:19.0Z"
    }
  }
}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPaths: []string{"/1.0/events", "/1.0/cluster/members"},
			// base64 encoded token from response body metadata.metadata.
			wantToken: "eyJzZXJ2ZXJfbmFtZSI6InNlcnZlcjEiLCJmaW5nZXJwcmludCI6ImZpbmdlcnByaW50IiwiYWRkcmVzc2VzIjpbIjEuMC4wLjEiLCIxLjAuMC4yIl0sInNlY3JldCI6InNlY3JldCIsImV4cGlyZXNfYXQiOiIyMDI1LTA2LTE3VDE1OjM5OjE5WiJ9",
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
			name:    "error - invalid cluster join token",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster/members
				{
					Value: response{
						statusCode: http.StatusOK,
						responseBody: []byte(`{
  "metadata": {
    "metadata": {
    }
  }
}`), // Join token content
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "Failed converting token operation to join token")
			},
			wantPaths: []string{"/1.0/events", "/1.0/cluster/members"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPaths []string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				response, _ := queue.Pop(t, &tc.response)
				w.WriteHeader(response.statusCode)
				_, _ = w.Write(response.responseBody)
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
			token, err := client.GetClusterJoinToken(ctx, target, "server1")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPaths, gotPaths)
			require.Equal(t, tc.wantToken, token)
			require.Empty(t, tc.response)
		})
	}
}

func TestClient_JoinCluster(t *testing.T) {
	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	certPEM, keyPEM := string(certPEMByte), string(keyPEMByte)

	type response struct {
		statusCode   int
		responseBody []byte
	}

	tests := []struct {
		name      string
		certPEM   string
		keyPEM    string
		response  []queue.Item[response]
		setup     func(*httptest.Server)
		assertErr require.ErrorAssertionFunc
		wantPaths []string
	}{
		{
			name:    "success",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster
				{
					Value: response{
						statusCode:   http.StatusOK,
						responseBody: []byte(`{"metadata":{}}`),
					},
				},
				// /1.0/operations//wait
				{
					Value: response{
						statusCode:   http.StatusOK,
						responseBody: []byte(`{"metadata":{}}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPaths: []string{"/1.0/events", "/1.0/cluster", "/1.0/operations//wait"},
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
			name:    "error - fail op.WaitContext",
			certPEM: certPEM,
			keyPEM:  keyPEM,
			response: []queue.Item[response]{
				// /1.0/events
				{
					Value: response{
						statusCode:   http.StatusForbidden,
						responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
					},
				},
				// /1.0/cluster
				{
					Value: response{
						statusCode:   http.StatusOK,
						responseBody: []byte(`{"metadata":{}}`),
					},
				},
				// /1.0/operations//wait
				{
					Value: response{
						statusCode:   http.StatusInternalServerError, // fail op.WaitContext
						responseBody: []byte(`{}`),
					},
				},
			},
			setup: func(_ *httptest.Server) {},

			assertErr: require.Error,
			wantPaths: []string{"/1.0/events", "/1.0/cluster", "/1.0/operations//wait"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotPaths []string
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				response, _ := queue.Pop(t, &tc.response)
				w.WriteHeader(response.statusCode)
				_, _ = w.Write(response.responseBody)
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

			cluster := provisioning.Server{}

			// Run test
			err := client.JoinCluster(ctx, target, "token", cluster)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPaths, gotPaths)
			require.Empty(t, tc.response)
		})
	}
}

func TestClient_UpdateNetworkConfig(t *testing.T) {
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
			response:   []byte(`{}`),
			setup:      func(_ *httptest.Server) {},

			assertErr: require.NoError,
			wantPath:  "/os/1.0/system/network",
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
				// OSData: api.OSData{
				// 	Network: incusosapi.SystemNetwork{
				// 		Config: &incusosapi.SystemNetworkConfig{
				// 			NTP: &incusosapi.SystemNetworkNTP{
				// 				Timeservers: []string{"0.pool.ntp.org"},
				// 			},
				// 		},
				// 	},
				// },
			}

			// Run test
			err := client.UpdateNetworkConfig(ctx, target)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantPath, gotPath)
		})
	}
}
