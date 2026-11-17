package localfs

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

type certificate struct {
	varRoot *os.Root
}

var _ system.CertificateRepo = certificate{}

// NewCertificate returns a certificate repo for the server certificate in varRoot.
func NewCertificate(varRoot *os.Root) certificate {
	return certificate{
		varRoot: varRoot,
	}
}

func (c certificate) GetServerCertificate(_ context.Context) ([]byte, error) {
	certificatePEM, err := c.varRoot.ReadFile(config.ServerCertificateFilename)
	if err != nil {
		return nil, fmt.Errorf("Failed to read %q: %w", config.ServerCertificateFilename, err)
	}

	return certificatePEM, nil
}

func (c certificate) GetServerKeyPair(ctx context.Context) (certificatePEM []byte, keyPEM []byte, _ error) {
	certificatePEM, err := c.GetServerCertificate(ctx)
	if err != nil {
		return nil, nil, err
	}

	keyPEM, err = c.varRoot.ReadFile(config.ServerKeyFilename)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to read %q: %w", config.ServerKeyFilename, err)
	}

	return certificatePEM, keyPEM, nil
}

func (c certificate) UpdateServerKeyPair(_ context.Context, certificatePEM []byte, keyPEM []byte) error {
	err := file.WriteFileAtomicIn(c.varRoot, config.ServerCertificateFilename, certificatePEM, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write %q: %w", config.ServerCertificateFilename, err)
	}

	err = file.WriteFileAtomicIn(c.varRoot, config.ServerKeyFilename, keyPEM, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write %q: %w", config.ServerKeyFilename, err)
	}

	return nil
}

func (c certificate) ValidateServerKeyPair(_ context.Context, dir string) error {
	return validateKeyPair(dir, config.ServerCertificateFilename, config.ServerKeyFilename)
}

func (c certificate) ValidateClientKeyPair(_ context.Context, dir string) error {
	return validateKeyPair(dir, config.ClientCertificateFilename, config.ClientKeyFilename)
}

func validateKeyPair(dir string, certificateFilename string, keyFilename string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", dir, err)
	}

	defer root.Close()

	certificatePEM, err := root.ReadFile(certificateFilename)
	if err != nil {
		return fmt.Errorf("Failed to read %q: %w", certificateFilename, err)
	}

	keyPEM, err := root.ReadFile(keyFilename)
	if err != nil {
		return fmt.Errorf("Failed to read %q: %w", keyFilename, err)
	}

	_, err = tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return fmt.Errorf("Failed to validate key pair %q and %q: %w", certificateFilename, keyFilename, err)
	}

	return nil
}
