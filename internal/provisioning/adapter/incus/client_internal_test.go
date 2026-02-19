package incus

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func TestClient_ClusterEndpointWithCA(t *testing.T) {
	const domainName = "cluster.company.com"

	tests := []struct {
		name                 string
		clusterConnectionURL *string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                 "success",
			clusterConnectionURL: ptr.To(fmt.Sprintf("https://%s/", domainName)),

			assertErr: require.NoError,
		},
		{
			name:                 "error - invalid cluster connection URL",
			clusterConnectionURL: ptr.To(":|\\"), // invalid

			assertErr: require.Error,
		},
		{
			name:                 "error - server ip is not present in cluster certificate",
			clusterConnectionURL: ptr.To("https://127.0.0.1/"),

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Create client certificate / key pair
			clientCertPEMByte, clientKeyPEMByte, err := incustls.GenerateMemCert(true, false)
			require.NoError(t, err)

			// Create CA pool from client certificate for use in the server to verify authentication of the client.
			caPool := x509.NewCertPool()
			caPool.AppendCertsFromPEM(clientCertPEMByte)

			// Create a server certificate chain with CA and leaf certificate for the server.
			// CA certificate is used by the client for verification of the authenticity of the server.
			serverCA, serverCert, serverKey := generateCertChain(t, domainName)

			serverTLSCert, err := tls.X509KeyPair(serverCert, serverKey)
			require.NoError(t, err)

			// Create http server. On successful request, an empty dummy response is returned with HTTP status 200.
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{
  "metadata": {}
}`))
			}))

			// Setup TLS for server:
			// - Require client certificate and verify it against the client certificate CA pool
			//   (which only contains a single certificate, which is the one used by the client).
			// - Set the servers TLS certificate.
			server.TLS = &tls.Config{
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    caPool,
				Certificates: []tls.Certificate{serverTLSCert},
			}

			server.StartTLS()
			defer server.Close()

			client := New(string(clientCertPEMByte), string(clientKeyPEMByte))
			client.clientCA = string(serverCA)

			// serverTarget without cluster certificate set, which is simulating the
			// case, where the server would have a publicly valid certificate
			// e.g. by using ACME.
			serverTarget := provisioning.Server{
				Name:                 "server01",
				ConnectionURL:        server.URL,
				Certificate:          string(serverCert),
				Cluster:              ptr.To("cluster"),
				ClusterConnectionURL: tc.clusterConnectionURL,
			}

			// Run test
			err = client.Ping(ctx, serverTarget)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func generateCertChain(t *testing.T, domainName string) (caCert []byte, cert []byte, key []byte) {
	t.Helper()

	// CA

	caPrivk, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Linux Containers"},
			CommonName:   "Test Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(60 * time.Minute),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caDERBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivk.PublicKey, caPrivk)
	require.NoError(t, err)

	caCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDERBytes})

	// Certificate

	privk, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	serialNumber, err = rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	certTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Linux Containers"},
			CommonName:   "Cluster",
		},

		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(10 * time.Minute),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{domainName},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, caTemplate, &privk.PublicKey, caPrivk)
	require.NoError(t, err)

	privateKey, err := x509.MarshalPKCS8PrivateKey(privk)
	require.NoError(t, err)

	cert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	key = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKey})

	return caCert, cert, key
}

func Test_mapIncusEventToLifecycleEvent(t *testing.T) {
	tests := []struct {
		name  string
		event incusapi.Event

		assertErr            require.ErrorAssertionFunc
		wantIsLifecycleEvent bool
		wantEvent            domain.LifecycleEvent
	}{
		{
			name: "success - create image",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleImageCreated,
					Source: "/1.0/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					Context: map[string]any{
						"type": "container",
					},
					Name:    "",
					Project: "",
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationCreate,
				ResourceType:         domain.ResourceTypeImage,
				LifecycleEventAction: "image-created",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
		},
		{
			name: "success - rename instance",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleInstanceRenamed,
					Source: "/1.0/instances/name-new",
					Context: map[string]any{
						"old_name": "name-old",
					},
					Name:    "name-new",
					Project: "default",
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationRename,
				ResourceType:         domain.ResourceTypeInstance,
				LifecycleEventAction: "instance-renamed",
				Source: domain.LifecycleSource{
					Name:        "name-new",
					ProjectName: "default",
					OldName:     "name-old",
				},
			},
		},
		{
			name: "success - delete storage-volume",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action:  incusapi.EventLifecycleStorageVolumeDeleted,
					Source:  "/1.0/storage-pools/default/volumes/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					Name:    "",
					Project: "",
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationDelete,
				ResourceType:         domain.ResourceTypeStorageVolume,
				LifecycleEventAction: "storage-volume-deleted",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ParentType:  "storage-pool",
					ParentName:  "default",
					Type:        "images",
					ProjectName: "default",
				},
			},
		},
		{
			name: "success - not a lifecycle event",
			event: func() incusapi.Event {
				return incusapi.Event{
					Type:      incusapi.EventTypeLogging,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(nil),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "error - invalid lifecycle metadata",
			event: func() incusapi.Event {
				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(`[]`), // array is invalid for event lifecycle metadata.
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.Error,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "success - not mapped lifecycle action",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: "warning-reset", // not mapped lifecycle action
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "error - invalid source URL",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleInstanceCreated,
					Source: ":|//", // invalid URL
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data), // array is invalid for event lifecycle metadata.
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.Error,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "success - delete storage-volume - with invalid type form context",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleStorageVolumeDeleted,
					Source: "/1.0/storage-pools/default/volumes/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					Context: map[string]any{
						"type": true, // invalid, string expected
					},
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationDelete,
				ResourceType:         domain.ResourceTypeStorageVolume,
				LifecycleEventAction: "storage-volume-deleted",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ParentType:  "storage-pool",
					ParentName:  "default",
					Type:        "images",
					ProjectName: "default",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run test
			event, isLifecycleEvent, err := mapIncusEventToLifecycleEvent(t.Context(), tc.event)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantIsLifecycleEvent, isLifecycleEvent)
			require.Equal(t, tc.wantEvent, event)
		})
	}
}

func Test_firstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string

		want string
	}{
		{
			name: "first",
			inputs: []string{
				"first",
				"",
				"last",
			},

			want: "first",
		},
		{
			name: "last",
			inputs: []string{
				"",
				"last",
			},

			want: "last",
		},
		{
			name:   "default from nil",
			inputs: nil,

			want: "",
		},
		{
			name: "default from only empty string",
			inputs: []string{
				"",
				"",
			},

			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstNonEmpty(tc.inputs...)

			require.Equal(t, tc.want, got)
		})
	}
}
