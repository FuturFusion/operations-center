package incus_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v6/shared/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

type clientPort interface {
	provisioning.ServerClientPort
	provisioning.ClusterClientPort
}

type methodTestSetEndpoint struct {
	name       string
	clientCall func(ctx context.Context, client clientPort, endpoint provisioning.Endpoint) (any, error)

	testCases []methodTestCase
}

type methodTestSetServer struct {
	name       string
	clientCall func(ctx context.Context, client clientPort, endpoint provisioning.Server) (any, error)

	testCases []methodTestCase
}

type methodTestCase struct {
	name     string
	response []queue.Item[response]

	assertErr    require.ErrorAssertionFunc
	wantPaths    []string
	assertBodies func(t *testing.T, gotBodies []string)
	assertResult func(t *testing.T, res any)
}

type response struct {
	statusCode   int
	responseBody []byte
}

func noResult(t *testing.T, res any) {
	t.Helper()
}

func TestClient_Endpoint(t *testing.T) {
	caPool, certPEM, keyPEM := setupCerts(t)

	methods := []methodTestSetEndpoint{
		{
			name: "Ping",
			clientCall: func(ctx context.Context, c clientPort, endpoint provisioning.Endpoint) (any, error) {
				return nil, c.Ping(ctx, endpoint)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /"},
				},
			},
		},

		{
			name: "GetClusterNodeNames",
			clientCall: func(ctx context.Context, client clientPort, endpoint provisioning.Endpoint) (any, error) {
				return client.GetClusterNodeNames(ctx, endpoint)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [ "https://127.0.0.1/cluster/members/one" ]
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Len(t, res, 1)
					},
					wantPaths: []string{"GET /1.0/cluster/members"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/cluster/members"},
				},
			},
		},
		{
			name: "GetClusterJoinToken",
			clientCall: func(ctx context.Context, client clientPort, endpoint provisioning.Endpoint) (any, error) {
				return client.GetClusterJoinToken(ctx, endpoint, "server1")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members
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

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/events", "POST /1.0/cluster/members"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						// base64 encoded token from response body metadata.metadata.
						wantToken := "eyJzZXJ2ZXJfbmFtZSI6InNlcnZlcjEiLCJmaW5nZXJwcmludCI6ImZpbmdlcnByaW50IiwiYWRkcmVzc2VzIjpbIjEuMC4wLjEiLCIxLjAuMC4yIl0sInNlY3JldCI6InNlY3JldCIsImV4cGlyZXNfYXQiOiIyMDI1LTA2LTE3VDE1OjM5OjE5WiJ9"
						require.Equal(t, wantToken, res)
					},
				},
				{
					name: "error - CreateClusterMember - unexpected status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /1.0/events", "POST /1.0/cluster/members"},
					assertResult: noResult,
				},
				{
					name: "error - invalid cluster join token",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// POST /1.0/cluster/members
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

					assertErr: func(tt require.TestingT, err error, a ...any) {
						require.ErrorContains(tt, err, "Failed converting token operation to join token")
					},
					wantPaths:    []string{"GET /1.0/events", "POST /1.0/cluster/members"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateClusterCertificate",
			clientCall: func(ctx context.Context, client clientPort, endpoint provisioning.Endpoint) (any, error) {
				return nil, client.UpdateClusterCertificate(ctx, endpoint, "new cert", "new key")
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /1.0/cluster/certificate"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /1.0/cluster/certificate"},
				},
			},
		},
	}

	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			ctx := context.Background()

			// endpointGetClientErr error - invalid key pair
			endpointGetClientErr(t, method, caPool, certPEM)

			// run regular test cases
			for _, tc := range method.testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Setup
					var gotPaths []string
					var gotBodies []string
					server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						gotPaths = append(gotPaths, fmt.Sprintf("%s %s", r.Method, r.URL.String()))

						body, _ := io.ReadAll(r.Body)
						gotBodies = append(gotBodies, string(body))

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

					client := incus.New(certPEM, keyPEM)

					serverCert := pem.EncodeToMemory(&pem.Block{
						Type:  "CERTIFICATE",
						Bytes: server.Certificate().Raw,
					})

					target := provisioning.Server{
						ConnectionURL: server.URL,
						Certificate:   string(serverCert),
					}

					// Run test
					retValue, err := method.clientCall(ctx, client, target)

					// Assert
					tc.assertErr(t, err)

					require.Equal(t, tc.wantPaths, gotPaths)

					if tc.assertResult != nil || retValue != nil {
						tc.assertResult(t, retValue)
					}

					if tc.assertBodies != nil {
						tc.assertBodies(t, gotBodies)
					}

					require.Empty(t, tc.response)
				})
			}
		})
	}
}

func endpointGetClientErr(t *testing.T, method methodTestSetEndpoint, caPool *x509.CertPool, certPEM string) {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.TLS = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}

	server.StartTLS()
	defer server.Close()

	client := incus.New(certPEM, certPEM) // invalid key

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})

	target := provisioning.Server{
		ConnectionURL: server.URL,
		Certificate:   string(serverCert),
	}

	_, err := method.clientCall(context.Background(), client, target)
	require.Error(t, err)
}

func TestClientServer(t *testing.T) {
	caPool, certPEM, keyPEM := setupCerts(t)

	methods := []methodTestSetServer{
		{
			name: "GetResources",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return client.GetResources(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "cpu": {
      "architecture": "x86_64"
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						want := api.HardwareData{
							Resources: incusapi.Resources{
								CPU: incusapi.ResourcesCPU{
									Architecture: "x86_64",
								},
							},
						}

						require.Equal(t, want, res)
					},
					wantPaths: []string{"GET /os/1.0/system/resources"},
				},
				{
					name: "error - resource data unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusInternalServerError,
								responseBody: []byte(http.StatusText(http.StatusInternalServerError)),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/system/resources"},
				},
				{
					name: "error - resource data invalid JSON",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /os/1.0/system/resources"},
				},
			},
		},
		{
			name: "GetOSData",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return client.GetOSData(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
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
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "encryption_recovery_keys": [ "very secret recovery key" ]
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						wantResources := api.OSData{
							Network: incusosapi.SystemNetwork{
								Config: &incusosapi.SystemNetworkConfig{
									DNS: &incusosapi.SystemNetworkDNS{
										Hostname: "foobar",
										Domain:   "local",
									},
								},
							},
							Security: incusosapi.SystemSecurity{
								Config: struct {
									EncryptionRecoveryKeys []string `json:"encryption_recovery_keys" yaml:"encryption_recovery_keys"`
								}{
									EncryptionRecoveryKeys: []string{"very secret recovery key"},
								},
							},
						}

						require.Equal(t, wantResources, res)
					},
				},
				{
					name: "error - network data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network"},
					assertResult: noResult,
				},
				{
					name: "error - network data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network"},
					assertResult: noResult,
				},
				{
					name: "error - security data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
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
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security"},
					assertResult: noResult,
				},
				{
					name: "error - security data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/network
						{
							Value: response{
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
						// GET /os/1.0/system/security
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": []
}`), // array for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetServerType",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return client.GetServerType(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/incus"
  ]
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						require.Equal(t, api.ServerTypeIncus, res)
					},
				},
				{
					name: "success - multiple applications",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/other-application",
    "/os/1.0/applications/incus",
    "/os/1.0/applications/more-application"
  ]
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/applications"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						require.Equal(t, api.ServerTypeIncus, res)
					},
				},
				{
					name: "error - network data unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "error - network data invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`), // object for metadata is invalid.
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "invalid application",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/invalid"
  ]
}`), // invalid application
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "invalid application (empty)",
					response: []queue.Item[response]{
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/os/1.0/applications/"
  ]
}`), // invalid application (empty)
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/applications"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "EnableOSServiceLVM",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.EnableOSService(ctx, target, "lvm", map[string]any{"enabled": true})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/services/lvm"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/services/lvm"},
				},
			},
		},
		{
			name: "SetServerConfig",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.SetServerConfig(ctx, target, map[string]string{
					"key": "value",
				})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// PUT /1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0", "PUT /1.0"},
					assertBodies: func(t *testing.T, gotBodies []string) {
						t.Helper()
						require.Contains(t, gotBodies[1], `"key":"value"`)
					},
				},
				{
					name: "error - GetServer - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0"},
				},
				{
					name: "error - UpdateServer - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// PUT /1.0
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0", "PUT /1.0"},
				},
			},
		},
		{
			name: "EnableCluster",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return client.EnableCluster(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
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

					assertErr: require.NoError,
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						require.Equal(t, "certificate", res)
					},
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "success - no certificate returned",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
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

					assertErr:    require.NoError,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "success - invalid type for certificate",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {}
}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
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

					assertErr:    require.NoError,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "error - UpdateCluster - unexpected http status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster"},
				},
				{
					name: "error - fail op.WaitContext",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode:   http.StatusInternalServerError, // fail op.WaitContext
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr:    require.Error,
					assertResult: noResult,
					wantPaths:    []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
			},
		},
		{
			name: "UpdateNetworkConfig",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.UpdateNetworkConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"PUT /os/1.0/system/network"},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"PUT /os/1.0/system/network"},
				},
			},
		},
		{
			name: "JoinCluster",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.JoinCluster(ctx, target, "token", provisioning.ClusterEndpoint{})
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
				{
					name: "error - UpdateCluster - unexpected status code",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster"},
				},
				{
					name: "error - fail op.WaitContext",
					response: []queue.Item[response]{
						// GET /1.0/events
						{
							Value: response{
								statusCode:   http.StatusForbidden,
								responseBody: []byte(`{"type": "error", "error_code": 403, "error": "websocket forbidden"}`), // Prevent the websocket listener.
							},
						},
						// PUT /1.0/cluster
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"metadata":{}}`),
							},
						},
						// GET /1.0/operations//wait?timeout=-1
						{
							Value: response{
								statusCode:   http.StatusInternalServerError, // fail op.WaitContext
								responseBody: []byte(`{}`),
							},
						},
					},

					assertErr: require.Error,
					wantPaths: []string{"GET /1.0/events", "PUT /1.0/cluster", "GET /1.0/operations//wait?timeout=-1"},
				},
			},
		},
	}

	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			ctx := context.Background()

			// getClient error - invalid key pair
			serverGetClientErr(t, method, caPool, certPEM)

			// run regular test cases
			for _, tc := range method.testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Setup
					var gotPaths []string
					var gotBodies []string
					server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						gotPaths = append(gotPaths, fmt.Sprintf("%s %s", r.Method, r.URL.String()))

						body, _ := io.ReadAll(r.Body)
						gotBodies = append(gotBodies, string(body))

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

					client := incus.New(certPEM, keyPEM)

					serverCert := pem.EncodeToMemory(&pem.Block{
						Type:  "CERTIFICATE",
						Bytes: server.Certificate().Raw,
					})

					target := provisioning.Server{
						Name:               "server01",
						ConnectionURL:      server.URL,
						Certificate:        string(serverCert),
						ClusterCertificate: ptr.To(string(serverCert)),
						OSData: api.OSData{
							Network: incusosapi.SystemNetwork{
								State: incusosapi.SystemNetworkState{
									Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
										"enp5s0": {
											Addresses: []string{"192.168.1.2"},
											Roles:     []string{"clustering"},
											LACP: &incusosapi.SystemNetworkLACPState{
												LocalMAC: "45:e3:51:39:0c:51",
											},
										},
									},
								},
							},
						},
					}

					// Run test
					retValue, err := method.clientCall(ctx, client, target)

					// Assert
					tc.assertErr(t, err)

					require.Equal(t, tc.wantPaths, gotPaths)

					if tc.assertResult != nil || retValue != nil {
						tc.assertResult(t, retValue)
					}

					if tc.assertBodies != nil {
						tc.assertBodies(t, gotBodies)
					}

					require.Empty(t, tc.response)
				})
			}
		})
	}
}

func serverGetClientErr(t *testing.T, method methodTestSetServer, caPool *x509.CertPool, certPEM string) {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.TLS = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}

	server.StartTLS()
	defer server.Close()

	client := incus.New(certPEM, certPEM) // invalid key

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})

	target := provisioning.Server{
		ConnectionURL: server.URL,
		Certificate:   string(serverCert),
	}

	_, err := method.clientCall(context.Background(), client, target)
	require.Error(t, err)
}

func setupCerts(t *testing.T) (caPool *x509.CertPool, certPEM string, keyPEM string) {
	t.Helper()

	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool = x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	return caPool, string(certPEMByte), string(keyPEMByte)
}
