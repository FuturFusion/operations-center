package system

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/lxc/incus/v7/shared/revert"
	incustls "github.com/lxc/incus/v7/shared/tls"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type environment interface {
	VarDir() string
	CacheDir() string
	IsIncusOS() bool
}

type systemService struct {
	env       environment
	serverSvc ProvisioningServerService
	cacheRepo CacheRepo

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
	cacheRepo CacheRepo,
	opts ...SystemServiceOption,
) *systemService {
	systemSvc := &systemService{
		env:       env,
		serverSvc: serverSvc,
		cacheRepo: cacheRepo,

		acmeUpdateCertificateFunc: acme.UpdateCertificate,
	}

	for _, opt := range opts {
		opt(systemSvc)
	}

	return systemSvc
}

func (s *systemService) GetCertificate(_ context.Context) (system.Certificate, error) {
	certificateFile := filepath.Join(s.env.VarDir(), config.ServerCertificateFilename)

	certificatePEM, err := os.ReadFile(certificateFile)
	if err != nil {
		return system.Certificate{}, fmt.Errorf("Failed to read %q: %w", certificateFile, err)
	}

	return certificateFromPEM(string(certificatePEM))
}

// certificateFromPEM returns the leaf certificate of a PEM encoded certificate
// chain together with its metadata.
func certificateFromPEM(certificatePEM string) (system.Certificate, error) {
	cert, err := api.DecodeCert([]byte(certificatePEM))
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
		return fmt.Errorf("Failed to validate key pair: %w", err)
	}

	certificateFile := filepath.Join(s.env.VarDir(), config.ServerCertificateFilename)
	keyFile := filepath.Join(s.env.VarDir(), config.ServerKeyFilename)

	currentServerCertificate, err := os.ReadFile(certificateFile)
	if err != nil {
		return fmt.Errorf("Failed to read %q: %w", certificateFile, err)
	}

	currentServerKey, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("Failed to read %q: %w", keyFile, err)
	}

	currentCertificate, err := tls.X509KeyPair(currentServerCertificate, currentServerKey)
	if err != nil {
		return fmt.Errorf("Failed to validate current key pair: %w", err)
	}

	currentServerCertificateFingerprint := incustls.CertFingerprint(currentCertificate.Leaf)
	certificateFingerprint := incustls.CertFingerprint(serverCertificate.Leaf)

	// Same certificate skip update.
	if currentServerCertificateFingerprint == certificateFingerprint {
		return nil
	}

	err = os.WriteFile(certificateFile, []byte(certificatePEM), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to persist %q: %w", certificateFile, err)
	}

	err = os.WriteFile(keyFile, []byte(keyPEM), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to persist %q: %w", keyFile, err)
	}

	reverter := revert.New()
	defer reverter.Fail()

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

	err = s.updateProviderConfigAll(ctx, map[string]string{"server_certificate": certificatePEM})
	if err != nil {
		return err
	}

	reverter.Success()

	return nil
}

func (s *systemService) TriggerCertificateRenew(ctx context.Context, force bool) (changed bool, _ error) {
	newCert, err := s.acmeUpdateCertificateFunc(ctx, s.env, config.GetSecurity().ACME, force)
	if err != nil {
		return false, fmt.Errorf("ACME server certificate renewal failed: %w", err)
	}

	if newCert == nil {
		return false, nil
	}

	err = s.UpdateCertificate(ctx, newCert.Certificate, newCert.Key)
	if err != nil {
		return false, fmt.Errorf("Update server certificate with ACME certificate/key failed: %w", err)
	}

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
