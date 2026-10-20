package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/letsencrypt/pebble/v2/ca"
	"github.com/letsencrypt/pebble/v2/db"
	"github.com/letsencrypt/pebble/v2/va"
	"github.com/letsencrypt/pebble/v2/wfe"
	"github.com/stretchr/testify/require"
)

// acmeProvider is a fake ACME server (Pebble), which is served in-process by
// the test. Neither the CA of its directory nor the CA of the issued
// certificates is in any default CA bundle.
type acmeProvider struct {
	DirectoryURL string

	// DirectoryCAPEM is the self signed certificate of the TLS listener of the
	// ACME directory.
	DirectoryCAPEM string

	// IssuerRootPEM is the root CA of the issued certificates.
	IssuerRootPEM string
	IssuerRoots   *x509.CertPool
}

func startACMEProvider(t *testing.T) *acmeProvider {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Pebble reads these when its components are created. Skip the challenge
	// validation, so Pebble needs no access to Operations Center, and disable
	// the random delays and the random rejection of nonces.
	t.Setenv("PEBBLE_VA_ALWAYS_VALID", "1")
	t.Setenv("PEBBLE_VA_NOSLEEP", "1")
	t.Setenv("PEBBLE_WFE_NONCEREJECT", "0")

	logOutput := io.Discard
	if debug {
		logOutput = os.Stderr
	}

	logger := log.New(logOutput, "pebble ", log.LstdFlags)

	store := db.NewMemoryStore()
	acmeCA := ca.New(logger, store, "", "rsa", 0, 1, map[string]ca.Profile{
		"default": {
			Description: "Operations Center end 2 end tests",
		},
	})
	acmeVA := va.New(logger, 80, 443, false, "", store)
	acmeWFE := wfe.New(logger, store, acmeVA, acmeCA, []string{"pebble.letsencrypt.org"}, false, false, 3, 5)

	issuerRoot := acmeCA.GetRootCert(0)
	require.NotNil(t, issuerRoot, "Failed to get the root CA of the ACME server")

	issuerRoots := x509.NewCertPool()
	issuerRoots.AddCert(issuerRoot.Cert)

	hostAddress := e2eHostAddress(t)
	tlsCert, directoryCAPEM := mustGenerateSelfSignedCertificate(t, hostAddress, "e2e-acme-server")

	// Bind on all the interfaces, since the address, at which the
	// OperationsCenter VM reaches the host, is derived separately.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err, "Failed to listen for the ACME server")

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "Failed to get the port of the ACME server listener")

	port := strconv.Itoa(tcpAddr.Port)
	server := &http.Server{
		Handler:           acmeWFE.Handler(),
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
			t.Errorf("Failed to shut down the ACME server: %v", err)
		}
	})

	go func() {
		err := server.ServeTLS(listener, "", "")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("ACME server stopped unexpectedly: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(10*time.Second))
	defer cancel()

	err = waitForTCPPort(ctx, t, net.JoinHostPort("127.0.0.1", port), 100*time.Millisecond)
	require.NoError(t, err, "ACME server did not come up")

	directoryURL := "https://" + net.JoinHostPort(hostAddress, port) + wfe.DirectoryPath

	t.Logf("ACME server is listening on %s", directoryURL)

	return &acmeProvider{
		DirectoryURL:   directoryURL,
		DirectoryCAPEM: directoryCAPEM,
		IssuerRootPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuerRoot.DER})),
		IssuerRoots:    issuerRoots,
	}
}
