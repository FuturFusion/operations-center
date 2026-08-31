package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/FuturFusion/incus-redfish-proxy/redfishproxy"
	"github.com/stretchr/testify/require"
)

// startRedfishProxy serves an in-process Redfish proxy for the given Incus
// instance on an ephemeral host port and returns the endpoint URL, at which the
// proxy is reachable from inside the OperationsCenter VM.
func startRedfishProxy(t *testing.T, instanceName string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	hostAddress := redfishProxyHostAddress(t)

	// The proxy needs to talk to the same Incus remote and project as the incus
	// CLI used by the tests, otherwise it does not find the instance.
	handler, err := redfishproxy.NewHandler(redfishproxy.Config{
		InstanceName: instanceName,
		Remote:       mustRun(t, `incus remote get-default`).OutputTrimmed(),
		Project:      mustRun(t, `incus project get-current`).OutputTrimmed(),
	})
	require.NoErrorf(t, err, "Failed to create Redfish proxy handler for %q", instanceName)

	tlsCert := mustGenerateRedfishProxyCertificate(t, hostAddress)

	// Bind on all the interfaces, since the address, at which the
	// OperationsCenter VM reaches the host, is derived separately.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err, "Failed to listen for the Redfish proxy")

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "Failed to get the port of the Redfish proxy listener")

	port := strconv.Itoa(tcpAddr.Port)
	server := &http.Server{
		Handler:           handler,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{tlsCert}},
		ReadHeaderTimeout: strechedTimeout(10 * time.Second),
	}

	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		err := server.Shutdown(ctx)
		if err != nil {
			t.Errorf("Failed to shut down the Redfish proxy for %q: %v", instanceName, err)
		}
	})

	go func() {
		err := server.ServeTLS(listener, "", "")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Redfish proxy for %q stopped unexpectedly: %v", instanceName, err)
		}
	}()

	endpoint := "https://" + net.JoinHostPort(hostAddress, port)

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(10*time.Second))
	defer cancel()

	err = waitForTCPPort(ctx, t, net.JoinHostPort("127.0.0.1", port), 100*time.Millisecond)
	require.NoErrorf(t, err, "Redfish proxy for %q did not come up", instanceName)

	mustAssertRedfishProxyReachable(t, endpoint, instanceName)

	t.Logf("Redfish proxy for %q is listening on %s", instanceName, endpoint)

	return endpoint
}

// mustGenerateRedfishProxyCertificate generates the self signed certificate
// presented by the Redfish proxy.
//
// Operations Center pins this certificate during the connection test and then
// derives the expected server name from it, preferring the first DNS SAN over
// the first IP SAN. The certificate therefore contains the address, at which the
// OperationsCenter VM reaches the proxy, as its only SAN.
func mustGenerateRedfishProxyCertificate(t *testing.T, hostAddress string) tls.Certificate {
	t.Helper()

	ip := net.ParseIP(hostAddress)
	require.NotNilf(t, ip, "Address of the Redfish proxy %q is not an IP address", hostAddress)

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err, "Failed to generate the key for the Redfish proxy")

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Operations Center end 2 end tests"},
			CommonName:   "incus-redfish-proxy",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{ip},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err, "Failed to create the certificate for the Redfish proxy")

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err, "Failed to load the certificate for the Redfish proxy")

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	require.NoError(t, err)

	err = cert.VerifyHostname(hostAddress)
	require.NoErrorf(t, err, "Certificate of the Redfish proxy is not valid for %q", hostAddress)

	return tlsCert
}

// mustAssertRedfishProxyReachable verifies, that the Redfish proxy answers the
// service root, which is the first resource requested by a Redfish client, and
// the computer system of the instance, which is the first resource requiring
// the proxy to talk to Incus. This distinguishes a broken proxy from a proxy,
// which is not reachable from inside the OperationsCenter VM.
func mustAssertRedfishProxyReachable(t *testing.T, endpoint string, instanceName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(10*time.Second))
	defer cancel()

	// The certificate is pinned by Operations Center, not by this smoke test.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint: gosec // self signed certificate of the test's own proxy.
		},
	}

	resources := []string{
		"/redfish/v1/",
		fmt.Sprintf("/redfish/v1/Systems/%s", instanceName),
	}

	for _, resource := range resources {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+resource, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoErrorf(t, err, "Redfish proxy for %q is not answering", instanceName)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		err = resp.Body.Close()
		require.NoError(t, err)

		require.Equalf(t, http.StatusOK, resp.StatusCode, "expect the Redfish proxy to serve %q, got: %s", resource, body)
	}
}

// redfishProxyHostAddress returns the address of the e2e host, at which the
// in-process Redfish proxy is reachable from inside the OperationsCenter VM.
func redfishProxyHostAddress(t *testing.T) string {
	t.Helper()

	if bmcProxyAddress != "" {
		return bmcProxyAddress
	}

	return e2eHostAddress(t)
}
