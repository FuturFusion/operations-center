package e2e

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

// acmeRenewal runs Operations Center with certificates from the in-process
// ACME server, whose CA certificates are in no default CA bundle. The CA
// certificates are added to the Operations Center host before the servers are
// deployed, so the seed of the servers contains them as well. Operations Center
// is reused by the following tests, so every change is reverted on cleanup.
func acmeRenewal(names []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t)
		defer stop()

		// Setup

		// A failed run without cleanup leaves the ACME config behind.
		err := resetSystemSecurityACMEWithContext(ctx, t, tmpDir)
		require.NoError(t, err, "Failed to reset the ACME security config")

		provider := startACMEProvider(t)

		mustAddOperationsCenterHostCustomCACerts(t, tmpDir, provider.DirectoryCAPEM, provider.IssuerRootPEM)

		mustBackupOperationsCenterCertificate(t, tmpDir)

		t.Cleanup(systemSecurityACMECleanup(t, tmpDir))

		// The cached ISO of the other tests lacks the CA certificates of this run.
		setupIncusOSWithTokenFreshISO(names)(ctx, t, tmpDir)

		// Runs first, so the other cleanups can use the CLI, even if the test
		// failed after a change of the certificate.
		t.Cleanup(func() {
			if noCleanup || (noCleanupOnError && t.Failed()) {
				return
			}

			// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
			ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
			defer cancel()

			err := retrustOperationsCenterRemoteWithContext(ctx, t)
			if err != nil {
				t.Logf("Failed to add the remote of the CLI again: %v", err)
			}
		})

		// Run test
		t.Log("Verify, that the seed added the CA certificates of the ACME server to the servers")
		for _, name := range names {
			resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server os system security show %s:`, cpuArch, name)

			var security incusosapi.SystemSecurity
			err := yaml.Unmarshal([]byte(resp.OutputTrimmed()), &security)
			require.NoError(t, err)

			require.Containsf(t, security.Config.CustomCACerts, provider.DirectoryCAPEM, "custom CA certificates of %q contain the CA of the ACME directory", name)
			require.Containsf(t, security.Config.CustomCACerts, provider.IssuerRootPEM, "custom CA certificates of %q contain the root CA of the ACME server", name)
		}

		initialFingerprint := mustServedOperationsCenterCertificateFingerprint(ctx, t)

		t.Log("Enable ACME, which triggers a renewal of the certificate")
		ocHostname := mustRun(t, `incus exec OperationsCenter -- hostname`).OutputTrimmed()
		mustSetSystemSecurityACME(ctx, t, tmpDir, ocHostname, provider.DirectoryURL)

		fingerprint := mustWaitOperationsCenterCertificate(ctx, t, provider.IssuerRoots, initialFingerprint)
		mustRetrustOperationsCenterRemote(ctx, t)

		for _, name := range names {
			mustWaitServerCertificatePinned(ctx, t, name, fingerprint)
		}

		mustWaitInventoryReady(ctx, t, names)

		t.Log("Force the renewal of the certificate")
		mustRun(t, `../bin/operations-center.linux.%s system certificate renew`, cpuArch)

		previousFingerprint := fingerprint
		fingerprint = mustWaitOperationsCenterCertificate(ctx, t, provider.IssuerRoots, previousFingerprint)
		mustRetrustOperationsCenterRemote(ctx, t)

		for _, name := range names {
			mustWaitServerCertificatePinned(ctx, t, name, fingerprint)
		}

		mustWaitInventoryReady(ctx, t, names)

		offlineServer := names[len(names)-1]
		onlineServers := names[:len(names)-1]

		t.Logf("Force the renewal of the certificate, while %s is offline", offlineServer)
		require.NoError(t, stopInstanceWithContext(ctx, t, offlineServer))

		// Without this, a server, that is back quickly, stays ready in Operations
		// Center and only the hourly poll would update its certificate.
		success, err := waitForSuccessWithTimeout(ctx, t, offlineServer+" offline", `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.server_status == "offline"'`, 7*time.Minute, cpuArch, offlineServer)
		require.NoError(t, err)
		require.Truef(t, success, "expect Operations Center to report %s as offline", offlineServer)

		mustRun(t, `../bin/operations-center.linux.%s system certificate renew`, cpuArch)

		previousFingerprint = fingerprint
		fingerprint = mustWaitOperationsCenterCertificate(ctx, t, provider.IssuerRoots, previousFingerprint)
		mustRetrustOperationsCenterRemote(ctx, t)

		for _, name := range onlineServers {
			mustWaitServerCertificatePinned(ctx, t, name, fingerprint)
		}

		require.NoError(t, startInstanceWithContext(ctx, t, offlineServer))
		mustWaitIncusOSReady(ctx, t, []string{offlineServer})
		mustWaitInventoryReady(ctx, t, []string{offlineServer})

		// Assert
		mustWaitServerCertificatePinned(ctx, t, offlineServer, fingerprint)
	}
}

// mustAddOperationsCenterHostCustomCACerts adds the CA certificates to the
// custom CA certificates of the IncusOS host of Operations Center. It restores
// the original security config of the host on cleanup.
func mustAddOperationsCenterHostCustomCACerts(t *testing.T, tmpDir string, caCertificatesPEM ...string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, `../bin/operations-center.linux.%s admin os system security show`, cpuArch)
	originalSecurityConfig := resp.OutputTrimmed()

	originalFilename := filepath.Join(tmpDir, "operations_center_host_security_original.yaml")
	err := os.WriteFile(originalFilename, []byte(originalSecurityConfig), 0o600)
	require.NoError(t, err)

	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s admin os system security edit < %s`, cpuArch, originalFilename)
		err := fmtRunErr(resp)
		if err != nil {
			t.Logf("Failed to restore the security config of the Operations Center host: %v", err)
		}
	})

	config := map[string]any{}
	err = yaml.Unmarshal([]byte(originalSecurityConfig), &config)
	require.NoError(t, err)

	securityConfig, ok := config["config"].(map[string]any)
	require.True(t, ok, "security config of the Operations Center host has a config section")

	var customCACerts []any

	existing, ok := securityConfig["custom_ca_certs"].([]any)
	if ok {
		customCACerts = existing
	}

	for _, caCertificatePEM := range caCertificatesPEM {
		customCACerts = append(customCACerts, caCertificatePEM)
	}

	securityConfig["custom_ca_certs"] = customCACerts

	configBody, err := yaml.Marshal(config)
	require.NoError(t, err)

	configFilename := filepath.Join(tmpDir, "operations_center_host_security.yaml")
	err = os.WriteFile(configFilename, configBody, 0o600)
	require.NoError(t, err)

	// No restart of Operations Center is required. lego reads the system CA
	// pool on every run and the seed is created from the security config.
	mustRun(t, `../bin/operations-center.linux.%s admin os system security edit < %s`, cpuArch, configFilename)
}

// mustBackupOperationsCenterCertificate copies the certificate and the key of
// Operations Center and restores them on cleanup.
func mustBackupOperationsCenterCertificate(t *testing.T, tmpDir string) {
	t.Helper()

	certificateFilename := filepath.Join(tmpDir, "operations_center_server.crt")
	keyFilename := filepath.Join(tmpDir, "operations_center_server.key")

	mustRun(t, `incus file pull OperationsCenter/var/lib/operations-center/server.crt %s`, certificateFilename)
	mustRun(t, `incus file pull OperationsCenter/var/lib/operations-center/server.key %s`, keyFilename)

	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(2*time.Minute))
		defer cancel()

		stop := timeTrack(t, "Operations Center certificate cleanup")
		defer stop()

		err := retrustOperationsCenterRemoteWithContext(ctx, t)
		if err != nil {
			t.Logf("Failed to add the remote of the CLI again: %v", err)
		}

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s system certificate set %s %s`, cpuArch, certificateFilename, keyFilename)
		err = fmtRunErr(resp)
		if err != nil {
			t.Logf("Failed to restore the certificate of Operations Center: %v", err)
		}

		err = retrustOperationsCenterRemoteWithContext(ctx, t)
		if err != nil {
			t.Logf("Failed to add the remote of the CLI again: %v", err)
		}
	})
}

func systemSecurityACMECleanup(t *testing.T, tmpDir string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "system security ACME cleanup")
		defer stop()

		err := retrustOperationsCenterRemoteWithContext(ctx, t)
		if err != nil {
			t.Logf("Failed to add the remote of the CLI again: %v", err)
		}

		err = resetSystemSecurityACMEWithContext(ctx, t, tmpDir)
		if err != nil {
			t.Logf("Failed to reset the ACME security config: %v", err)
		}
	}
}

func mustSetSystemSecurityACME(ctx context.Context, t *testing.T, tmpDir string, domain string, caURL string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	securityPutFilename := filepath.Join(tmpDir, "system_security_put.json")

	resp := mustRunWithContext(ctx, t, `../bin/operations-center.linux.%[1]s system security show -f json | jq -ce --arg domain '%[2]s' --arg ca_url '%[3]s' '.acme = { domain: $domain, email: "e2e@example.com", ca_url: $ca_url, agree_tos: true, challenge: "HTTP-01", http_challenge_address: "", provider: "", provider_environment: [], provider_resolvers: [] }' > %[4]s`, cpuArch, domain, caURL, securityPutFilename)
	require.NoError(t, fmtRunErr(resp), "Failed to assemble the security config")

	err := applySystemSecurityWithContext(ctx, t, securityPutFilename)
	require.NoErrorf(t, err, "Failed to set the ACME security config with CA URL %q", caURL)
}

// resetSystemSecurityACMEWithContext disables ACME. With an empty config, a
// renewal of the certificate does nothing.
func resetSystemSecurityACMEWithContext(ctx context.Context, t *testing.T, tmpDir string) error {
	t.Helper()

	securityPutFilename := filepath.Join(tmpDir, "system_security_put.json")

	resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s system security show -f json | jq -ce '.acme = { domain: "", email: "", ca_url: "", agree_tos: false, challenge: "", http_challenge_address: "", provider: "", provider_environment: [], provider_resolvers: [] }' > %s`, cpuArch, securityPutFilename)

	err := fmtRunErr(resp)
	if err != nil {
		return fmt.Errorf("Failed to assemble the security config: %w", err)
	}

	return applySystemSecurityWithContext(ctx, t, securityPutFilename)
}

// mustRetrustOperationsCenterRemote adds the remote of the CLI again, so the
// CLI pins the certificate, which Operations Center serves now.
func mustRetrustOperationsCenterRemote(ctx context.Context, t *testing.T) {
	t.Helper()

	err := retrustOperationsCenterRemoteWithContext(ctx, t)
	require.NoError(t, err, "Failed to add the remote of the CLI again")
}

// retrustOperationsCenterRemoteWithContext is mustRetrustOperationsCenterRemote
// for cleanups, where t.Context() is already cancelled.
func retrustOperationsCenterRemoteWithContext(ctx context.Context, t *testing.T) error {
	t.Helper()

	address, err := operationsCenterIPAddressWithContext(ctx, t)
	if err != nil {
		return err
	}

	operationsCenterAddress := fmt.Sprintf("https://%s/", net.JoinHostPort(address, "8443"))

	for _, command := range []string{
		`../bin/operations-center.linux.%[1]s remote remove e2e-test`,
		`../bin/operations-center.linux.%[1]s remote add --accept-certificate e2e-test %[2]s`,
		`../bin/operations-center.linux.%[1]s remote switch e2e-test`,
	} {
		err := fmtRunErr(runWithContext(ctx, t, command, cpuArch, operationsCenterAddress))
		if err != nil {
			return err
		}
	}

	return nil
}

func mustServedOperationsCenterCertificateFingerprint(ctx context.Context, t *testing.T) string {
	t.Helper()

	chain, err := servedOperationsCenterCertificate(ctx, t)
	require.NoError(t, err, "Failed to get the certificate of Operations Center")

	return certificateFingerprint(chain[0].Raw)
}

// mustWaitOperationsCenterCertificate waits until Operations Center presents a
// certificate, which is issued by roots and differs from the certificate with
// notFingerprint. It returns the fingerprint of the new certificate.
func mustWaitOperationsCenterCertificate(ctx context.Context, t *testing.T, roots *x509.CertPool, notFingerprint string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(3*time.Minute))
	defer cancel()

	var lastErr error
	for {
		chain, err := servedOperationsCenterCertificate(ctx, t)
		if err == nil {
			intermediates := x509.NewCertPool()
			for _, cert := range chain[1:] {
				intermediates.AddCert(cert)
			}

			_, err = chain[0].Verify(x509.VerifyOptions{
				Roots:         roots,
				Intermediates: intermediates,
			})

			fingerprint := certificateFingerprint(chain[0].Raw)
			if err == nil && fingerprint != notFingerprint {
				t.Logf("Operations Center presents the ACME certificate %s issued by %q", fingerprint, chain[0].Issuer)

				return fingerprint
			}

			if err == nil {
				err = fmt.Errorf("Operations Center still presents the certificate %s", fingerprint)
			}
		}

		lastErr = err

		select {
		case <-ctx.Done():
			logVMDebugInfo(t, "OperationsCenter")
			require.FailNowf(t, "Operations Center does not present a new ACME certificate", "last error: %v", lastErr)

		case <-time.After(1 * time.Second):
		}
	}
}

// mustWaitServerCertificatePinned waits until the server pins the certificate
// of Operations Center with the fingerprint.
func mustWaitServerCertificatePinned(ctx context.Context, t *testing.T, name string, fingerprint string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(2*time.Minute))
	defer cancel()

	var gotFingerprint string
	for {
		resp := runQuietWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server os system provider show %s:`, cpuArch, name)
		if resp.Success() {
			var provider incusosapi.SystemProvider
			err := yaml.Unmarshal([]byte(resp.OutputTrimmed()), &provider)
			require.NoError(t, err)

			block, _ := pem.Decode([]byte(provider.Config.Config["server_certificate"]))
			if block != nil {
				gotFingerprint = certificateFingerprint(block.Bytes)
			}

			if gotFingerprint == fingerprint {
				t.Logf("Server %q pins the certificate %s", name, fingerprint)

				return
			}
		}

		select {
		case <-ctx.Done():
			logVMDebugInfo(t, "OperationsCenter", name)
			require.FailNowf(t, "Server does not pin the new certificate of Operations Center", "server %q, want %s, got %q", name, fingerprint, gotFingerprint)

		case <-time.After(5 * time.Second):
		}
	}
}

func certificateFingerprint(der []byte) string {
	sum := sha256.Sum256(der)

	return hex.EncodeToString(sum[:])
}
