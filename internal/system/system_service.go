package system

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maniartech/signals"
)

type environment interface {
	VarDir() string
}

type systemService struct {
	env                     environment
	serverCertificateUpdate signals.Signal[tls.Certificate]
}

var _ SystemService = &systemService{}

func NewSystemService(env environment, serverCertificateUpdate signals.Signal[tls.Certificate]) *systemService {
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
