package system

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"github.com/lxc/incus/v7/shared/revert"
	incustls "github.com/lxc/incus/v7/shared/tls"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type environment interface {
	VarDir() string
	CacheDir() string
	IsIncusOS() bool
}

type systemService struct {
	env             environment
	serverSvc       ProvisioningServerService
	clusterSvc      ProvisioningClusterService
	cacheRepo       CacheRepo
	databaseRepo    DatabaseRepo
	backupRepo      BackupRepo
	certificateRepo CertificateRepo

	// requestRestart asks the daemon to restart.
	requestRestart func()

	// restoreMu is held by a restore and never released after a successful one.
	restoreMu sync.Mutex

	acmeUpdateCertificateFunc func(
		ctx context.Context,
		fsEnv interface {
			VarDir() string
			CacheDir() string
		},
		cfg system.SecurityACME,
		force bool,
	) (*system.CertificatePost, error)
}

var _ SystemService = &systemService{}

type SystemServiceOption func(s *systemService)

func NewSystemService(
	env environment,
	serverSvc ProvisioningServerService,
	clusterSvc ProvisioningClusterService,
	cacheRepo CacheRepo,
	databaseRepo DatabaseRepo,
	backupRepo BackupRepo,
	certificateRepo CertificateRepo,
	requestRestart func(),
	opts ...SystemServiceOption,
) *systemService {
	systemSvc := &systemService{
		env:             env,
		serverSvc:       serverSvc,
		clusterSvc:      clusterSvc,
		cacheRepo:       cacheRepo,
		databaseRepo:    databaseRepo,
		backupRepo:      backupRepo,
		certificateRepo: certificateRepo,
		requestRestart:  requestRestart,

		acmeUpdateCertificateFunc: acme.UpdateCertificate,
	}

	for _, opt := range opts {
		opt(systemSvc)
	}

	return systemSvc
}

func (s *systemService) GetCertificate(ctx context.Context) (system.Certificate, error) {
	certificatePEM, err := s.certificateRepo.GetServerCertificate(ctx)
	if err != nil {
		return system.Certificate{}, fmt.Errorf("Failed to read the server certificate: %w", err)
	}

	return certificateFromPEM(string(certificatePEM))
}

// certificateFromPEM returns the leaf certificate of a PEM encoded certificate
// chain together with its metadata.
func certificateFromPEM(certificatePEM string) (system.Certificate, error) {
	cert, err := certificate.Decode([]byte(certificatePEM))
	if err != nil {
		return system.Certificate{}, err
	}

	ipAddresses := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ipAddresses = append(ipAddresses, ip.String())
	}

	return system.Certificate{
		Certificate: certificatePEM,
		Fingerprint: incustls.CertFingerprint(cert),
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		DNSNames:    cert.DNSNames,
		IPAddresses: ipAddresses,
	}, nil
}

func (s *systemService) UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) (err error) {
	serverCertificate, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return domain.NewValidationErrf("Failed to validate key pair: %v", err)
	}

	err = validateCertificateChain(serverCertificate)
	if err != nil {
		return err
	}

	currentServerCertificate, currentServerKey, err := s.certificateRepo.GetServerKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("Failed to read the server key pair: %w", err)
	}

	currentCertificate, err := tls.X509KeyPair(currentServerCertificate, currentServerKey)
	if err != nil {
		return fmt.Errorf("Failed to validate current key pair: %w", err)
	}

	// Same certificate chain, skip update.
	if slices.EqualFunc(currentCertificate.Certificate, serverCertificate.Certificate, bytes.Equal) {
		return nil
	}

	reverter := revert.New()
	defer reverter.Fail()

	// Restore the certificate and the key, even if the update wrote only one of
	// them. This keeps the certificate and the key matching.
	reverter.Add(func() {
		revertErr := s.certificateRepo.UpdateServerKeyPair(ctx, currentServerCertificate, currentServerKey)
		if revertErr != nil {
			err = errors.Join(err, fmt.Errorf("Failed to restore the server key pair: %w", revertErr))
		}
	})

	err = s.certificateRepo.UpdateServerKeyPair(ctx, []byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("Failed to persist the server key pair: %w", err)
	}

	// Restart openfga application, if it is present as secondary application
	// on IncusOS.
	if s.env.IsIncusOS() {
		servers, err := s.serverSvc.GetAllWithFilter(ctx, provisioning.ServerFilter{
			Type: new(api.ServerTypeOperationsCenter),
		})
		if err != nil {
			return fmt.Errorf("Failed to get operations-center server entry: %w", err)
		}

		if len(servers) != 1 {
			//domain-errors:internal Violated invariant, nothing the user can do about it.
			return fmt.Errorf("Failed to get operations-center server entry, expected 1 entry, got %d", len(servers))
		}

		hasOpenFGA := false
		for _, app := range servers[0].VersionData.Applications {
			if app.Name == "openfga" {
				hasOpenFGA = true
				break
			}
		}

		if hasOpenFGA {
			err = s.serverSvc.RestartApplication(ctx, servers[0].Name, "openfga")
			if err != nil {
				return fmt.Errorf("Failed to restart OpenFGA application: %w", err)
			}
		}
	}

	reverter.Add(func() {
		revertErr := lifecycle.ServerCertificateUpdateSignal.TryEmit(ctx, currentCertificate)
		if revertErr != nil {
			err = errors.Join(err, fmt.Errorf("Failed to revert back to the original certificate: %w ", revertErr))
		}
	})

	// Notify services about new certificate, which also causes the http listener
	// to switch to the new certificate, which is necessary for the the provider
	// updates to be successful.
	err = lifecycle.ServerCertificateUpdateSignal.TryEmit(ctx, serverCertificate)
	if err != nil {
		return fmt.Errorf("Failed to update certificate: %w", err)
	}

	s.pushServerCertificateAll(ctx, certificatePEM)

	reverter.Success()

	return nil
}

// pushServerCertificateAll updates the server certificate in the provider
// config of all servers. Failures do not stop the rollout. The polling of the
// servers retries the update for the servers, that failed.
func (s *systemService) pushServerCertificateAll(ctx context.Context, certificatePEM string) {
	servers, err := s.serverSvc.GetAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to get servers for server certificate update, retry on next poll", logger.Err(err))
		return
	}

	for _, server := range servers {
		if server.Type == api.ServerTypeOperationsCenter || server.Status == api.ServerStatusUnregistered {
			continue
		}

		providerConfig, err := s.serverSvc.GetSystemProvider(ctx, server.Name)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get system provider for server certificate update, retry on next poll", slog.String("server", server.Name), logger.Err(err))
			continue
		}

		if providerConfig.Config.Config == nil {
			providerConfig.Config.Config = map[string]string{}
		}

		providerConfig.Config.Config["server_certificate"] = certificatePEM

		err = s.serverSvc.UpdateSystemProvider(ctx, server.Name, providerConfig)
		if err != nil {
			slog.WarnContext(ctx, "Failed to update server certificate in system provider, retry on next poll", slog.String("server", server.Name), logger.Err(err))
		}
	}
}

func (s *systemService) TriggerCertificateRenew(ctx context.Context, force bool) (changed bool, _ error) {
	cfg := config.GetSecurity().ACME

	newCert, err := s.acmeUpdateCertificateFunc(ctx, s.env, cfg, force)
	if err != nil {
		return false, fmt.Errorf("ACME server certificate renewal failed: %w", err)
	}

	if newCert == nil {
		if !acme.IsEnabled(cfg) {
			slog.InfoContext(ctx, "ACME server certificate renewal skipped, ACME is not configured")
		}

		return false, nil
	}

	err = s.UpdateCertificate(ctx, newCert.Certificate, newCert.Key)
	if err != nil {
		return false, fmt.Errorf("Update server certificate with ACME certificate/key failed: %w", err)
	}

	slog.InfoContext(ctx, "ACME server certificate renewed", slog.String("domain", cfg.Domain))

	return true, nil
}

func (s *systemService) GetNetworkConfig(_ context.Context) system.Network {
	return config.GetNetwork()
}

func (s *systemService) UpdateNetworkConfig(ctx context.Context, newConfig system.NetworkPut) error {
	// Make sure the new config is valid.
	newConfig, err := config.NetworkSetDefaults(newConfig)
	if err != nil {
		return err
	}

	err = config.ValidateNetworkConfig(system.Network{
		NetworkPut: newConfig,
	})
	if err != nil {
		return err
	}

	if newConfig.OperationsCenterAddress != config.GetNetwork().OperationsCenterAddress {
		err = s.updateProviderConfigAll(ctx, map[string]string{"server_url": newConfig.OperationsCenterAddress})
		if err != nil {
			return err
		}
	}

	err = config.UpdateNetwork(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update network configuration: %w", err)
	}

	return nil
}

func (s *systemService) updateProviderConfigAll(ctx context.Context, cfg map[string]string) (deferErr error) {
	servers, err := s.serverSvc.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Update provider config, failed to get all servers: %w", err)
	}

	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		// Don't update the system provider config for Operations Center.
		if server.Type == api.ServerTypeOperationsCenter {
			continue
		}

		// Don't update the system provider config for unregistered servers.
		if server.Status == api.ServerStatusUnregistered {
			continue
		}

		oldProviderConfig, err := s.serverSvc.GetSystemProvider(ctx, server.Name)
		if err != nil {
			return fmt.Errorf("Failed to get system provider for %q: %w", server.Name, err)
		}

		providerConfig := oldProviderConfig

		if providerConfig.Config.Config == nil {
			providerConfig.Config.Config = map[string]string{}
		}

		maps.Copy(providerConfig.Config.Config, cfg)

		err = s.serverSvc.UpdateSystemProvider(ctx, server.Name, providerConfig)
		if err != nil {
			return fmt.Errorf("Failed to update provider config of %q: %w", server.Name, err)
		}

		reverter.Add(func() {
			err := s.serverSvc.UpdateSystemProvider(ctx, server.Name, oldProviderConfig)
			if err != nil {
				deferErr = errors.Join(deferErr, fmt.Errorf("Failed to revert provider config of %q: %w", server.Name, err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s *systemService) GetSecurityConfig(_ context.Context) system.Security {
	return config.GetSecurity()
}

func (s *systemService) UpdateSecurityConfig(ctx context.Context, newConfig system.SecurityPut) error {
	err := config.UpdateSecurity(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update security configuration: %w", err)
	}

	return nil
}

func (s *systemService) GetSettingsConfig(_ context.Context) system.Settings {
	return config.GetSettings()
}

func (s *systemService) UpdateSettingsConfig(ctx context.Context, newConfig system.SettingsPut) error {
	err := config.UpdateSettings(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update security configuration: %w", err)
	}

	return nil
}

func (s *systemService) GetUpdatesConfig(_ context.Context) system.Updates {
	return config.GetUpdates()
}

func (s *systemService) UpdateUpdatesConfig(ctx context.Context, newConfig system.UpdatesPut) error {
	err := config.UpdateUpdates(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update updates configuration: %w", err)
	}

	return nil
}

func (s *systemService) CleanCache(ctx context.Context) error {
	err := s.cacheRepo.CleanupAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to clean cache: %w", err)
	}

	return nil
}

// validateCertificateChain verifies, that the certificates are provided leaf
// first, followed by the intermediate certificates in order.
func validateCertificateChain(cert tls.Certificate) error {
	certs := make([]*x509.Certificate, 0, len(cert.Certificate))
	for i, der := range cert.Certificate {
		parsedCert, err := x509.ParseCertificate(der)
		if err != nil {
			return domain.NewValidationErrf("Invalid certificate chain, failed to parse certificate at position %d: %v", i, err)
		}

		certs = append(certs, parsedCert)
	}

	for i := range len(certs) - 1 {
		err := certs[i].CheckSignatureFrom(certs[i+1])
		if err != nil {
			return domain.NewValidationErrf("Invalid certificate chain, certificate %q is not signed by %q: %v", certs[i].Subject, certs[i+1].Subject, err)
		}
	}

	return nil
}
