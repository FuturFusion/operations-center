package incus_test

import (
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

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

type methodTestSet struct {
	name       string
	clientCall func(ctx context.Context, client incus.Client, server provisioning.Server) (any, error)

	// wantPathsPrefix compares the recorded request paths against wantPaths by
	// prefix instead of by exact equality. The generated inventory test cases
	// depend on this, since their expected paths omit the query string.
	wantPathsPrefix bool

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

func mustJSONMarshal(t *testing.T, in any) []byte {
	t.Helper()

	out, err := json.Marshal(in)
	require.NoError(t, err)

	return out
}

// runMethodTestSets exercises every method test set against an Incus server
// stub, which answers with the queued responses of the respective test case.
func runMethodTestSets(t *testing.T, methods []methodTestSet) {
	t.Helper()

	caPool, certPEM, keyPEM := setupCerts(t)

	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// getClientErr error - invalid key pair
			getClientErr(t, method, caPool, certPEM)

			// run regular test cases
			for _, tc := range method.testCases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

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

					client := incus.New(securitytls.NewClientKeypairHolder(certPEM, keyPEM), incus.WithSkipGetServer(true))

					serverCert := pem.EncodeToMemory(&pem.Block{
						Type:  "CERTIFICATE",
						Bytes: server.Certificate().Raw,
					})

					target := newTestServer(server.URL, string(serverCert))

					// Run test
					retValue, err := method.clientCall(ctx, client, target)

					// Assert
					tc.assertErr(t, err)

					if method.wantPathsPrefix {
						for i := range tc.wantPaths {
							require.True(t, strings.HasPrefix(gotPaths[i], tc.wantPaths[i]), "want prefix %q, got %q", tc.wantPaths[i], gotPaths[i])
						}
					} else {
						require.Equal(t, tc.wantPaths, gotPaths)
					}

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

// getClientErr asserts that every method of the test set reports an error, if
// no connection to the Incus server can be established.
func getClientErr(t *testing.T, method methodTestSet, caPool *x509.CertPool, certPEM string) {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.TLS = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}

	server.StartTLS()
	defer server.Close()

	client := incus.New(securitytls.NewClientKeypairHolder(certPEM, certPEM), incus.WithSkipGetServer(true)) // invalid key

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})

	target := newTestServer(server.URL, string(serverCert))

	_, err := method.clientCall(context.Background(), client, target)
	require.Error(t, err)
}

// newTestServer returns the server the method test sets are run against. It
// carries a cluster and a network fixture, since some of the methods under test
// depend on them.
func newTestServer(connectionURL string, serverCert string) provisioning.Server {
	return provisioning.Server{
		Name:               "server01",
		ConnectionURL:      connectionURL,
		Certificate:        new(serverCert),
		Cluster:            new("cluster"),
		ClusterCertificate: new(serverCert),
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
}

func TestClient_Endpoint(t *testing.T) {
	t.Parallel()

	methods := []methodTestSet{
		{
			name: "Ping",
			clientCall: func(ctx context.Context, c incus.Client, endpoint provisioning.Server) (any, error) {
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
			clientCall: func(ctx context.Context, client incus.Client, endpoint provisioning.Server) (any, error) {
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
			clientCall: func(ctx context.Context, client incus.Client, endpoint provisioning.Server) (any, error) {
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
			clientCall: func(ctx context.Context, client incus.Client, endpoint provisioning.Server) (any, error) {
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
			clientCall: func(ctx context.Context, c incus.Client, endpoint provisioning.Server) (any, error) {
				return nil, c.SystemFactoryReset(
					ctx,
					endpoint,
					false,
					provisioning.TokenImageSeedConfigs{
						Install: api.SeedInstall{
							Version: "1",
						},
						Network: api.SeedNetwork{
							Version: "1",
						},
						Update: api.SeedUpdate{
							Version: "1",
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

	runMethodTestSets(t, methods)
}

func setupCerts(t *testing.T) (caPool *x509.CertPool, certPEM string, keyPEM string) {
	t.Helper()

	certPEMByte, keyPEMByte, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	caPool = x509.NewCertPool()
	caPool.AppendCertsFromPEM(certPEMByte)

	return caPool, string(certPEMByte), string(keyPEMByte)
}

func TestClient_input_validation(t *testing.T) {
	t.Parallel()

	client := incus.New(securitytls.NewClientKeypairHolder("", ""), incus.WithSkipGetServer(true))

	_, err := client.GetSystem(t.Context(), provisioning.Server{}, "invalid/resource")
	require.ErrorContains(t, err, "must not contain forward slashes")

	err = client.UpdateSystem(t.Context(), provisioning.Server{}, "invalid/resource", nil)
	require.ErrorContains(t, err, "must not contain forward slashes")

	err = client.TriggerSystemAction(t.Context(), provisioning.Server{}, "invalid/resource", "action", nil)
	require.ErrorContains(t, err, "must not contain forward slashes")
	err = client.TriggerSystemAction(t.Context(), provisioning.Server{}, "resource", "invalid/action", nil)
	require.ErrorContains(t, err, "must not contain forward slashes")
}
