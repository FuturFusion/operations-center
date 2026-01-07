package incus

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
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

			instanceServer := New(string(clientCertPEMByte), string(clientKeyPEMByte))
			client, ok := instanceServer.(serverClient)
			require.True(t, ok)
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
