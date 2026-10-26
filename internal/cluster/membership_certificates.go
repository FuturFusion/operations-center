package cluster

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/security/acme"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// prepareServerCertificate replaces the server certificate with a self-signed one before this member becomes clustered.
// If requested and ACME is configured, it also prepares an ACME issued cluster certificate. The returned function reverts the changes.
func (m *membership) prepareServerCertificate(ctx context.Context, requestACMEClusterCertificate bool) (func(), error) {
	oldCert := m.serverCert.Load()

	if requestACMEClusterCertificate {
		// If ACME is not configured or fails, the cluster certificate is self-signed until the next renewal.
		newCert, err := acme.UpdateCertificate(ctx, m.env, config.GetSecurity().ACME, nil, true)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get ACME cluster certificate, using a self-signed one", logger.Err(err))
		}

		if newCert != nil {
			keyPair, err := tls.X509KeyPair([]byte(newCert.Certificate), []byte(newCert.Key))
			if err != nil {
				return nil, fmt.Errorf("Failed to validate ACME cluster certificate: %w", err)
			}

			m.mu.Lock()
			m.pendingClusterCert = cowsqltls.NewCertInfo(keyPair, nil, nil)
			m.mu.Unlock()
		}
	}

	certFile := filepath.Join(m.env.VarDir(), config.ServerCertificateFilename)
	keyFile := filepath.Join(m.env.VarDir(), config.ServerKeyFilename)

	oldCertPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("Failed to read server certificate: %w", err)
	}

	oldKeyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("Failed to read server key: %w", err)
	}

	revert := func() {
		m.mu.Lock()
		m.pendingClusterCert = nil
		m.mu.Unlock()

		err := errors.Join(os.WriteFile(certFile, oldCertPEM, 0o600), os.WriteFile(keyFile, oldKeyPEM, 0o600))
		if err != nil {
			slog.ErrorContext(ctx, "Failed to restore server certificate", logger.Err(err))
		}

		m.serverCert.Store(oldCert)
	}

	err = errors.Join(os.Remove(certFile), os.Remove(keyFile))
	if err != nil {
		revert()
		return nil, fmt.Errorf("Failed to remove server certificate: %w", err)
	}

	newCert, err := incustls.KeyPairAndCA(m.env.VarDir(), "server", incustls.CertServer, true)
	if err != nil {
		revert()
		return nil, fmt.Errorf("Failed to generate self-signed server certificate: %w", err)
	}

	slog.InfoContext(ctx, "Replaced server certificate with a self-signed one for the communication within the cluster")

	m.serverCert.Store(newCert)

	return revert, nil
}

// takePendingClusterCertificate returns the cluster certificate prepared by prepareServerCertificate, if any.
func (m *membership) takePendingClusterCertificate() cowsqltls.CertInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	certInfo := m.pendingClusterCert
	m.pendingClusterCert = nil

	return certInfo
}

// applyClientKeypair persists the client keypair and lets the daemon use it.
func (m *membership) applyClientKeypair(ctx context.Context, certificatePEM string, keyPEM string) error {
	_, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return domain.NewValidationErrf("Failed to validate client keypair: %v", err)
	}

	err = securitytls.WriteClientKeypair(m.env.VarDir(), certificatePEM, keyPEM)
	if err != nil {
		return err
	}

	return lifecycle.ClientKeypairUpdateSignal.TryEmit(ctx, lifecycle.ClientKeypairMessage{Certificate: certificatePEM, Key: keyPEM})
}

// generateClientKeypair replaces the client keypair with a newly generated one.
func (m *membership) generateClientKeypair(ctx context.Context) error {
	certificatePEM, keyPEM, err := securitytls.GenerateClientKeypair(m.env.VarDir())
	if err != nil {
		return err
	}

	return lifecycle.ClientKeypairUpdateSignal.TryEmit(ctx, lifecycle.ClientKeypairMessage{Certificate: certificatePEM, Key: keyPEM})
}
