package incus_test

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	incusapi "github.com/lxc/incus/v7/shared/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func TestClient_UpdateCluster_certificateReplaced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientCall func(ctx context.Context, client incus.Client, target provisioning.Server) (string, error)

		wantClusterCertificate func(clusterCertPEM string) string
	}{
		{
			name: "EnableCluster",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (string, error) {
				return client.EnableCluster(ctx, target)
			},

			wantClusterCertificate: func(clusterCertPEM string) string {
				return clusterCertPEM
			},
		},
		{
			name: "JoinCluster",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server) (string, error) {
				clusterEndpoint := provisioning.Server{
					ConnectionURL: "https://192.0.2.1:8443",
					Certificate:   new("cluster certificate"),
				}

				return "", client.JoinCluster(ctx, target, "token", "192.0.2.2:8443", clusterEndpoint, nil)
			},

			wantClusterCertificate: func(clusterCertPEM string) string {
				return ""
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			caPool, certPEM, keyPEM := setupCerts(t)

			clusterCertPEM, clusterKeyPEM, err := incustls.GenerateMemCert(false, true)
			require.NoError(t, err)

			clusterCert, err := tls.X509KeyPair(clusterCertPEM, clusterKeyPEM)
			require.NoError(t, err)

			var certificateReplaced atomic.Bool
			var connectionsAfterReplacement atomic.Int32
			eventStreams := make(chan *websocket.Conn, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /1.0/events":
					serveEventStream(t, w, r, eventStreams)

				case "PUT /1.0/cluster":
					// Incus replaces its certificate while the cluster operation runs.
					certificateReplaced.Store(true)

					_, _ = w.Write(mustJSONMarshal(t, incusapi.Response{
						Type:     incusapi.AsyncResponse,
						Metadata: mustJSONMarshal(t, incusapi.Operation{ID: "op-id", StatusCode: incusapi.Running}),
					}))

					sendEvents(t, <-eventStreams, []string{
						string(mustJSONMarshal(t, incusapi.Event{
							Type: incusapi.EventTypeOperation,
							Metadata: mustJSONMarshal(t, incusapi.Operation{
								ID:         "op-id",
								StatusCode: incusapi.Success,
								Metadata:   map[string]any{"certificate": string(clusterCertPEM)},
							}),
						})),
					})

				default:
					t.Errorf("Unexpected request %s %s", r.Method, r.URL.String())
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew && certificateReplaced.Load() {
					connectionsAfterReplacement.Add(1)
				}
			}

			server.TLS = &tls.Config{
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  caPool,
			}

			// GetCertificate is not called for handshakes without server name, so use
			// GetConfigForClient to replace the certificate for every new connection.
			server.TLS.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
				if !certificateReplaced.Load() {
					return nil, nil
				}

				config := server.TLS.Clone()
				config.Certificates = []tls.Certificate{clusterCert}

				return config, nil
			}

			server.StartTLS()
			defer server.Close()

			client := incus.New(certPEM, keyPEM, incus.WithSkipGetServer(true))

			serverCert := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: server.Certificate().Raw,
			})

			target := newTestServer(server.URL, string(serverCert))
			target.Cluster = nil
			target.ClusterCertificate = nil

			// Run test
			gotClusterCertificate, err := tc.clientCall(t.Context(), client, target)

			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.wantClusterCertificate(string(clusterCertPEM)), gotClusterCertificate)
			require.Zero(t, connectionsAfterReplacement.Load(), "new connections fail the verification against the pinned server certificate")
		})
	}
}
