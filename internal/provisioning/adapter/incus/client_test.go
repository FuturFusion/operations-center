package incus_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v6/shared/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
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
		{
			name: "SystemFactoryReset",
			clientCall: func(ctx context.Context, c clientPort, endpoint provisioning.Endpoint) (any, error) {
				return nil, c.SystemFactoryReset(
					ctx,
					endpoint,
					false,
					provisioning.TokenImageSeedConfigs{
						Install: map[string]any{
							"key": "value",
						},
						Network: map[string]any{
							"key": "value",
						},
						Update: map[string]any{
							"key": "value",
						},
					},
					api.TokenProviderConfig{},
				)
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
					wantPaths: []string{"POST /os/1.0/system/:factory-reset"},
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
					wantPaths: []string{"POST /os/1.0/system/:factory-reset"},
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
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "pools": [
        {
          "name": "some pool"
        }
      ]
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security", "GET /os/1.0/system/storage"},
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
								Config: incusosapi.SystemSecurityConfig{
									EncryptionRecoveryKeys: []string{"very secret recovery key"},
								},
							},
							Storage: incusosapi.SystemStorage{
								Config: incusosapi.SystemStorageConfig{
									Pools: []incusosapi.SystemStoragePool{
										{
											Name: "some pool",
										},
									},
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
				{
					name: "error - storage data unexpected http status code",
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
						// GET /os/1.0/system/storage
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security", "GET /os/1.0/system/storage"},
					assertResult: noResult,
				},
				{
					name: "error - storage data invalid JSON",
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
						// GET /os/1.0/system/storage
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
					wantPaths:    []string{"GET /os/1.0/system/network", "GET /os/1.0/system/security", "GET /os/1.0/system/storage"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "GetVersionData",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return client.GetVersionData(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {
      "channel": "stable"
    },
    "state": {
      "needs_reboot": true
    }
  }
}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /os/1.0/system/update"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()
						wantResources := api.ServerVersionData{
							OS: api.OSVersionData{
								Name:        "IncusOS",
								Version:     "202511041601",
								VersionNext: "202512210545",
								NeedsReboot: true,
							},
							Applications: []api.ApplicationVersionData{
								{
									Name:    "incus",
									Version: "202511041601",
								},
							},
							UpdateChannel: "stable",
						}

						require.Equal(t, wantResources, res)
					},
				},
				{
					name: "error - os version unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0"},
					assertResult: noResult,
				},
				{
					name: "error - os version invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
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
					wantPaths:    []string{"GET /os/1.0"},
					assertResult: noResult,
				},
				{
					name: "error - applications unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "error - applications invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
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
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications"},
					assertResult: noResult,
				},
				{
					name: "error - application incus unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus"},
					assertResult: noResult,
				},
				{
					name: "error - application incus invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
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
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus"},
					assertResult: noResult,
				},
				{
					name: "error - update unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/update
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /os/1.0/system/update"},
					assertResult: noResult,
				},
				{
					name: "error - update invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "environment": {
      "hostname": "af94e64e-1993-41b6-8f10-a8eebb828fce",
      "os_name": "IncusOS",
      "os_version": "202511041601",
      "os_version_next": "202512210545"
    }
  }
}`),
							},
						},
						// GET /os/1.0/applications
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": [
    "/1.0/applications/incus"
  ]
}`),
							},
						},
						// GET /os/1.0/applications/incus
						{
							Value: response{
								statusCode: http.StatusOK,
								responseBody: []byte(`{
  "metadata": {
    "config": {},
    "state": {
      "initialized": true,
      "version": "202511041601"
    }
  }
}`),
							},
						},
						// GET /os/1.0/system/update
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
					wantPaths:    []string{"GET /os/1.0", "GET /os/1.0/applications", "GET /os/1.0/applications/incus", "GET /os/1.0/system/update"},
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
			name: "UpdateStorageConfig",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.UpdateStorageConfig(ctx, target)
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
					wantPaths: []string{"PUT /os/1.0/system/storage"},
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
					wantPaths: []string{"PUT /os/1.0/system/storage"},
				},
			},
		},
		{
			name: "GetProviderConfig",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return client.GetProviderConfig(ctx, target)
			},
			testCases: []methodTestCase{
				{
					name: "success",
					response: []queue.Item[response]{
						// GET /os/1.0/system/provider
						{
							Value: response{
								statusCode:   http.StatusOK,
								responseBody: []byte(`{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":{"config":{"name":"operations-center","config":{"server_certificate":"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n","server_token":"5df55e4e-3bfb-46c8-94c3-b04b56f9e3ba","server_url":"https://192.168.1.200:8443"}},"state":{"registered":true}}}`),
							},
						},
					},

					assertErr: require.NoError,
					wantPaths: []string{"GET /os/1.0/system/provider"},
					assertResult: func(t *testing.T, res any) {
						t.Helper()

						wantProviderConfig := provisioning.ServerSystemProvider{
							Config: incusosapi.SystemProviderConfig{
								Name: "operations-center",
								Config: map[string]string{
									"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
									"server_token":       "5df55e4e-3bfb-46c8-94c3-b04b56f9e3ba",
									"server_url":         "https://192.168.1.200:8443",
								},
							},
							State: incusosapi.SystemProviderState{
								Registered: true,
							},
						}

						require.Equal(t, wantProviderConfig, res)
					},
				},
				{
					name: "error - unexpected http status code",
					response: []queue.Item[response]{
						// GET /os/1.0/system/provider
						{
							Value: response{
								statusCode: http.StatusInternalServerError,
							},
						},
					},

					assertErr:    require.Error,
					wantPaths:    []string{"GET /os/1.0/system/provider"},
					assertResult: noResult,
				},
				{
					name: "error - provider config invalid JSON",
					response: []queue.Item[response]{
						// GET /os/1.0/system/provider
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
					wantPaths:    []string{"GET /os/1.0/system/provider"},
					assertResult: noResult,
				},
			},
		},
		{
			name: "UpdateProviderConfig",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.UpdateProviderConfig(ctx, target, incusosapi.SystemProvider{
					Config: incusosapi.SystemProviderConfig{
						Config: map[string]string{
							"server_url": "https://new-operations-center:8443",
						},
					},
				})
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
					wantPaths: []string{"PUT /os/1.0/system/provider"},
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
					wantPaths: []string{"PUT /os/1.0/system/provider"},
				},
			},
		},
		{
			name: "Poweroff",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.Poweroff(ctx, target)
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
					wantPaths: []string{"POST /os/1.0/system/:poweroff"},
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
					wantPaths: []string{"POST /os/1.0/system/:poweroff"},
				},
			},
		},
		{
			name: "Reboot",
			clientCall: func(ctx context.Context, client clientPort, target provisioning.Server) (any, error) {
				return nil, client.Reboot(ctx, target)
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
					wantPaths: []string{"POST /os/1.0/system/:reboot"},
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
					wantPaths: []string{"POST /os/1.0/system/:reboot"},
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestClientServer_SubscribeLifecycleEvents(t *testing.T) {
	caPool, certPEM, keyPEM := setupCerts(t)

	readyContext := func() (context.Context, func()) {
		return context.WithCancel(context.Background())
	}

	cancelledContext := func() (context.Context, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel the context already now.
		return ctx, cancel
	}

	noLogAssert := func(t *testing.T, logBuf *bytes.Buffer) {
		t.Helper()
	}

	logContains := func(want string) func(t *testing.T, logBuf *bytes.Buffer) {
		return func(t *testing.T, logBuf *bytes.Buffer) {
			t.Helper()

			// Give logs a little bit of time to be processed.
			for range 5 {
				if strings.Contains(logBuf.String(), want) {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			require.Contains(t, logBuf.String(), want)
		}
	}

	tests := []struct {
		name              string
		getCtx            func() (context.Context, func())
		blockEventChannel bool
		handler           func(done chan struct{}) func(http.ResponseWriter, *http.Request)
		clientCertPEM     string
		clientKeyPEM      string

		assertErr        require.ErrorAssertionFunc
		assertErrChanErr require.ErrorAssertionFunc
		wantEvent        domain.LifecycleEvent
		assertLog        func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:   "success one event",
			getCtx: readyContext,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					<-done
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			wantEvent: domain.LifecycleEvent{
				Operation:    domain.LifecycleOperationCreate,
				ResourceType: domain.ResourceTypeImage,
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
			assertLog: noLogAssert,
		},
		{
			name:   "error - getClient",
			getCtx: readyContext,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  certPEM, // invalid value

			assertErr: require.Error,
		},
		{
			name:   "error - GetEventsAllProjects",
			getCtx: readyContext,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr: require.Error,
		},
		{
			name:   "error - invalid lifecycle event",
			getCtx: readyContext,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					err = conn.WriteJSON(incusapi.Event{
						Type:      incusapi.EventTypeLifecycle,
						Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
						Metadata:  json.RawMessage([]byte("[]")), // invalid, object expected.
						Location:  "none",
						Project:   "default",
					}) // not a valid lifecycle event.
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					<-done
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			wantEvent: domain.LifecycleEvent{
				Operation:    domain.LifecycleOperationCreate,
				ResourceType: domain.ResourceTypeImage,
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
			assertLog: logContains("Failed to map incus event to lifecycle event"),
		},
		{
			name:   "error - unsupported lifecycle event resource type",
			getCtx: readyContext,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					err = conn.WriteJSON(createLifecycleEvent(t, "unsupported-action")) // unsupported action
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					<-done
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			wantEvent: domain.LifecycleEvent{
				Operation:    domain.LifecycleOperationCreate,
				ResourceType: domain.ResourceTypeImage,
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
			assertLog: noLogAssert,
		},
		{
			name:              "handle event - cancelled context",
			getCtx:            cancelledContext,
			blockEventChannel: true,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					// Since channel read operations in select pick a random case, we might
					// need multiple attempts until hitting the ctx.Done() case.
					for {
						err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
						if err != nil {
							t.Errorf("Failed to write event: %v", err)
							return
						}

						select {
						case <-done:
						default:
						}
					}
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			assertLog:        noLogAssert,
		},
		{
			name:   "error - webserver disconnect websocket immediately",
			getCtx: readyContext,
			handler: func(done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					// End the websocket connection right after it is established.
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.Error,
			assertLog:        noLogAssert,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			done := make(chan struct{})

			ctx, cancel := tc.getCtx()
			defer cancel()

			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			server := httptest.NewUnstartedServer(http.HandlerFunc(tc.handler(done)))
			server.TLS = &tls.Config{
				NextProtos: []string{"h2", "http/1.1"},
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  caPool,
			}

			server.StartTLS()
			defer server.Close()

			client := incus.New(tc.clientCertPEM, tc.clientKeyPEM)

			serverCert := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: server.Certificate().Raw,
			})

			target := provisioning.Server{
				ConnectionURL: server.URL,
				Certificate:   string(serverCert),
			}

			// Run test
			events, errChan, err := client.SubscribeLifecycleEvents(ctx, target)
			tc.assertErr(t, err)
			if err != nil {
				// we don't have any websocket connection at this point, so skip the rest of the assertions.
				return
			}

			if tc.blockEventChannel {
				events = nil
			}

			var event domain.LifecycleEvent
			var errChanErr error

			select {
			case event = <-events:
			case errChanErr = <-errChan:
			case <-ctx.Done():
			case <-t.Context().Done():
				t.Fatal("Test context cancelled before test ended")

			case <-time.After(5000 * time.Millisecond):
				t.Error("Test timeout reached before test ended")
			}

			close(done)

			// Assert
			tc.assertErrChanErr(t, errChanErr)
			require.Equal(t, tc.wantEvent, event)
			tc.assertLog(t, logBuf)
		})
	}
}

func createLifecycleEvent(t *testing.T, action string) incusapi.Event {
	t.Helper()

	data, err := json.Marshal(incusapi.EventLifecycle{
		Action: action,
		Source: "/1.0/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
		Context: map[string]any{
			"type": "container",
		},
		Requestor: nil,
		Name:      "",
		Project:   "",
	})
	require.NoError(t, err)

	return incusapi.Event{
		Type:      incusapi.EventTypeLifecycle,
		Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
		Metadata:  json.RawMessage(data),
		Location:  "none",
		Project:   "default",
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
