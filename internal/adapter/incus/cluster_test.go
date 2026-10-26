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

	joinCluster := func(ctx context.Context, client incus.Client, target provisioning.Server, clusterCertPEM string) (string, error) {
		clusterEndpoint := provisioning.Server{
			ConnectionURL: "https://192.0.2.1:8443",
			Certificate:   new(clusterCertPEM),
		}

		return "", client.JoinCluster(ctx, target, "token", "192.0.2.2:8443", clusterEndpoint, nil)
	}

	tests := []struct {
		name            string
		clientCall      func(ctx context.Context, client incus.Client, target provisioning.Server, clusterCertPEM string) (string, error)
		dropEventStream bool

		wantClusterCertificate          func(clusterCertPEM string) string
		wantConnectionsAfterReplacement int32
	}{
		{
			name: "EnableCluster",
			clientCall: func(ctx context.Context, client incus.Client, target provisioning.Server, _ string) (string, error) {
				return client.EnableCluster(ctx, target)
			},

			wantClusterCertificate: func(clusterCertPEM string) string {
				return clusterCertPEM
			},
		},
		{
			name:       "JoinCluster",
			clientCall: joinCluster,

			wantClusterCertificate: func(clusterCertPEM string) string {
				return ""
			},
		},
		{
			name:            "JoinCluster - recovered after lost event listener",
			clientCall:      joinCluster,
			dropEventStream: true,

			wantClusterCertificate: func(clusterCertPEM string) string {
				return ""
			},
			// Only the recovery connects anew, its first attempt with the cluster certificate succeeds.
			wantConnectionsAfterReplacement: 1,
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

					eventStream := <-eventStreams
					if tc.dropEventStream {
						_ = eventStream.Close()
						return
					}

					sendEvents(t, eventStream, []string{
						string(mustJSONMarshal(t, incusapi.Event{
							Type: incusapi.EventTypeOperation,
							Metadata: mustJSONMarshal(t, incusapi.Operation{
								ID:         "op-id",
								StatusCode: incusapi.Success,
								Metadata:   map[string]any{"certificate": string(clusterCertPEM)},
							}),
						})),
					})

				case "GET /1.0/operations/op-id/wait":
					_, _ = w.Write(mustJSONMarshal(t, incusapi.Response{
						Type:     incusapi.SyncResponse,
						Metadata: mustJSONMarshal(t, incusapi.Operation{ID: "op-id", StatusCode: incusapi.Success}),
					}))

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
			gotClusterCertificate, err := tc.clientCall(t.Context(), client, target, string(clusterCertPEM))

			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.wantClusterCertificate(string(clusterCertPEM)), gotClusterCertificate)
			require.Equal(t, tc.wantConnectionsAfterReplacement, connectionsAfterReplacement.Load(), "new connections after the certificate replacement")
		})
	}
}
