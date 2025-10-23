package system

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maniartech/signals"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type environment interface {
	VarDir() string
}

type systemService struct {
	env                     environment
	serverCertificateUpdate signals.Signal[tls.Certificate]
}

var _ SystemService = &systemService{}

func NewSystemService(
	env environment,
	serverCertificateUpdate signals.Signal[tls.Certificate],
) *systemService {
	return &systemService{
		env:                     env,
		serverCertificateUpdate: serverCertificateUpdate,
	}
}

func (s *systemService) UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error {
	serverCertificate, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("Failed to validate key pair: %w", err)
	}

	certificateFile := filepath.Join(s.env.VarDir(), "server.crt")
	err = os.WriteFile(certificateFile, []byte(certificatePEM), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to persist %q: %w", certificateFile, err)
	}

	keyFile := filepath.Join(s.env.VarDir(), "server.key")
	err = os.WriteFile(keyFile, []byte(keyPEM), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to persist %q: %w", keyFile, err)
	}

	s.serverCertificateUpdate.Emit(ctx, serverCertificate)

	return nil
}

func (s *systemService) GetNetworkConfig(_ context.Context) system.SystemNetwork {
	return config.GetNetwork()
}

func (s *systemService) UpdateNetworkConfig(ctx context.Context, newConfig system.SystemNetworkPut) error {
	err := config.UpdateNetwork(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update network configuration: %w", err)
	}

	return nil
}

func (s *systemService) GetSecurityConfig(_ context.Context) system.SystemSecurity {
	return config.GetSecurity()
}

func (s *systemService) UpdateSecurityConfig(ctx context.Context, newConfig system.SystemSecurityPut) error {
	err := config.UpdateSecurity(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update security configuration: %w", err)
	}

	return nil
}

func (s *systemService) GetUpdatesConfig(_ context.Context) system.SystemUpdates {
	return config.GetUpdates()
}

func (s *systemService) UpdateUpdatesConfig(ctx context.Context, newConfig system.SystemUpdatesPut) error {
	err := config.UpdateUpdates(ctx, newConfig)
	if err != nil {
		return fmt.Errorf("Failed to update updates configuration: %w", err)
	}

	return nil
}
