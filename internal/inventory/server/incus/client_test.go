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

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/inventory/server/incus"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
)

type methodTestSet struct {
	name       string
	clientCall func(ctx context.Context, client inventory.ServerClient, endpoint provisioning.Endpoint) (any, error)

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

func TestClient(t *testing.T) {
	caPool, certPEM, keyPEM := setupCerts(t)

	methods := []methodTestSet{
		{
			name: "Ping",
			clientCall: func(ctx context.Context, c inventory.ServerClient, endpoint provisioning.Endpoint) (any, error) {
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
	}

	methods = appendTestCases(t, methods)

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

					client := incus.New(certPEM, keyPEM, incus.WithSkipGetServer(true))

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

					for i := range tc.wantPaths {
						require.True(t, strings.HasPrefix(gotPaths[i], tc.wantPaths[i]), "want prefix %q, got %q", tc.wantPaths[i], gotPaths[i])
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

func TestClient_HasExtension(t *testing.T) {
	caPool, certPEM, keyPEM := setupCerts(t)

	tests := []struct {
		name         string
		argExtension string
		response     response
		want         bool
	}{
		{
			name:         "success",
			argExtension: "foobar",
			response: response{
				statusCode: http.StatusOK,
				responseBody: []byte(`{
  "metadata": {
    "api_extensions": [
      "foobar"
    ]
  }
}`),
			},
			want: true,
		},
		{
			name:         "extension not found",
			argExtension: "foobar",
			response: response{
				statusCode: http.StatusOK,
				responseBody: []byte(`{
  "metadata": {
    "api_extensions": []
  }
}`),
			},
			want: false,
		},
		{
			name:         "error",
			argExtension: "foobar",
			response: response{
				statusCode: http.StatusInternalServerError,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.response.statusCode)
				_, _ = w.Write(tc.response.responseBody)
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
			got := client.HasExtension(t.Context(), target, tc.argExtension)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}

func endpointGetClientErr(t *testing.T, method methodTestSet, caPool *x509.CertPool, certPEM string) {
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
