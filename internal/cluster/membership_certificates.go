package cluster

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"

	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/security/acme"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// prepareServerCertificate replaces the server certificate with a self-signed one before this member becomes clustered.
// If requested and ACME is configured, it also prepares an ACME issued cluster certificate. The returned function reverts the changes.
func (m *membership) prepareServerCertificate(ctx context.Context, requestACMEClusterCertificate bool) (func(), error) {
	oldCert := m.serverCert.Load()

	if requestACMEClusterCertificate {
		// If ACME is not configured or fails, the cluster certificate is self-signed until the next renewal.
		newCert, err := acme.UpdateCertificate(ctx, m.env, config.GetSecurity().ACME, nil, true)
		if err != nil {
			// The warning stays until the ACME renewal succeeds.
			m.warnings.Emit(ctx, warning.NewWarningFromError(api.WarningTypeACMECertificateUpdateFailed, warning.ScopeACMECertificateUpdate, err, "Failed to get an ACME cluster certificate, using a self-signed one"))
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

	oldCertPEM, err := m.varRoot.ReadFile(config.ServerCertificateFilename)
	if err != nil {
		return nil, fmt.Errorf("Failed to read server certificate: %w", err)
	}

	oldKeyPEM, err := m.varRoot.ReadFile(config.ServerKeyFilename)
	if err != nil {
		return nil, fmt.Errorf("Failed to read server key: %w", err)
	}

	revert := func() {
		m.mu.Lock()
		m.pendingClusterCert = nil
		m.mu.Unlock()

		err := errors.Join(m.varRoot.WriteFile(config.ServerCertificateFilename, oldCertPEM, 0o600), m.varRoot.WriteFile(config.ServerKeyFilename, oldKeyPEM, 0o600))
		if err != nil {
			slog.ErrorContext(ctx, "Failed to restore server certificate", logger.Err(err))
		}

		m.serverCert.Store(oldCert)
	}

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, true)
	if err != nil {
		revert()
		return nil, fmt.Errorf("Failed to generate self-signed server certificate: %w", err)
	}

	err = errors.Join(m.varRoot.WriteFile(config.ServerCertificateFilename, certPEM, 0o644), m.varRoot.WriteFile(config.ServerKeyFilename, keyPEM, 0o600))
	if err != nil {
		revert()
		return nil, fmt.Errorf("Failed to write self-signed server certificate: %w", err)
	}

	newCert, err := incustls.KeyPairFromRaw(certPEM, keyPEM)
	if err != nil {
		revert()
		return nil, fmt.Errorf("Failed to load self-signed server certificate: %w", err)
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

	err = securitytls.WriteClientKeypair(m.varRoot, certificatePEM, keyPEM)
	if err != nil {
		return err
	}

	return lifecycle.ClientKeypairUpdateSignal.TryEmit(ctx, lifecycle.ClientKeypairMessage{Certificate: certificatePEM, Key: keyPEM})
}

// generateClientKeypair replaces the client keypair with a newly generated one.
func (m *membership) generateClientKeypair(ctx context.Context) error {
	certificatePEM, keyPEM, err := securitytls.GenerateClientKeypair(m.varRoot)
	if err != nil {
		return err
	}

	return lifecycle.ClientKeypairUpdateSignal.TryEmit(ctx, lifecycle.ClientKeypairMessage{Certificate: certificatePEM, Key: keyPEM})
}
