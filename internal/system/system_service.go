package system

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lxc/incus/v6/shared/revert"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/internal/acme"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/shared/api"
)

type environment interface {
	VarDir() string
	CacheDir() string
}

type systemService struct {
	env                     environment
	serverCertificateUpdate signals.Signal[tls.Certificate]
	serverSvc               ProvisioningServerService

	acmeUpdateCertificateFunc func(
		ctx context.Context,
		fsEnv interface {
			VarDir() string
			CacheDir() string
		},
		cfg api.SystemSecurityACME,
		force bool,
	) (*api.SystemCertificatePost, error)
}

var _ SystemService = &systemService{}

type SystemServiceOption func(s *systemService)

func NewSystemService(
	env environment,
	serverCertificateUpdate signals.Signal[tls.Certificate],
	serverSvc ProvisioningServerService,
	opts ...SystemServiceOption,
) *systemService {
	systemSvc := &systemService{
		env:                     env,
		serverCertificateUpdate: serverCertificateUpdate,
		serverSvc:               serverSvc,

		acmeUpdateCertificateFunc: acme.UpdateCertificate,
	}

	for _, opt := range opts {
		opt(systemSvc)
	}

	return systemSvc
}

func (s *systemService) UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error {
	serverCertificate, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("Failed to validate key pair: %w", err)
	}

	certificateFile := filepath.Join(s.env.VarDir(), "server.crt")
	keyFile := filepath.Join(s.env.VarDir(), "server.key")

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

	// Notify services about new certificate, which also causes the http listener
	// to switch to the new certificate, which is necessary for the the provider
	// updates to be successful.
	s.serverCertificateUpdate.Emit(ctx, serverCertificate)
	reverter.Add(func() {
		s.serverCertificateUpdate.Emit(ctx, currentCertificate)
	})

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

func (s *systemService) GetNetworkConfig(_ context.Context) api.SystemNetwork {
	return config.GetNetwork()
}

func (s *systemService) UpdateNetworkConfig(ctx context.Context, newConfig api.SystemNetworkPut) error {
	// Make sure the new config is valid.
	newConfig, err := config.NetworkSetDefaults(newConfig)
	if err != nil {
		return err
	}

	err = config.ValidateNetworkConfig(api.SystemNetwork{
		SystemNetworkPut: newConfig,
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

		oldProviderConfig, err := s.serverSvc.GetSystemProvider(ctx, server.Name)
		if err != nil {
			return fmt.Errorf("Failed to get system provider for %q: %w", server.Name, err)
		}

		providerConfig := oldProviderConfig

		if providerConfig.Config.Config == nil {
			providerConfig.Config.Config = map[string]string{}
		}

		for key, value := range cfg {
			providerConfig.Config.Config[key] = value
		}

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

func (s *systemService) GetSecurityConfig(_ context.Context) api.SystemSecurity {
	return config.GetSecurity()
}

func (s *systemService) UpdateSecurityConfig(ctx context.Context, newConfig api.SystemSecurityPut) error {
	err := config.UpdateSecurity(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update security configuration: %w", err)
	}

	return nil
}

func (s *systemService) GetSettingsConfig(_ context.Context) api.SystemSettings {
	return config.GetSettings()
}

func (s *systemService) UpdateSettingsConfig(ctx context.Context, newConfig api.SystemSettingsPut) error {
	err := config.UpdateSettings(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update security configuration: %w", err)
	}

	return nil
}

func (s *systemService) GetUpdatesConfig(_ context.Context) api.SystemUpdates {
	return config.GetUpdates()
}

func (s *systemService) UpdateUpdatesConfig(ctx context.Context, newConfig api.SystemUpdatesPut) error {
	err := config.UpdateUpdates(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update updates configuration: %w", err)
	}

	return nil
}
