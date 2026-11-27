package localfs

import (
	"context"
	"fmt"
	"os"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

type certificate struct {
	varRoot   *os.Root
	clustered func() bool
}

var _ system.CertificateRepo = certificate{}

// CertificateOption configures the certificate repo.
type CertificateOption func(c *certificate)

// WithClustered stores the certificate shared by the cluster members instead, while clustered reports true.
func WithClustered(clustered func() bool) CertificateOption {
	return func(c *certificate) {
		c.clustered = clustered
	}
}

// NewCertificate returns a certificate repo for the server certificate in varRoot.
func NewCertificate(varRoot *os.Root, opts ...CertificateOption) certificate {
	c := certificate{
		varRoot: varRoot,
	}

	for _, opt := range opts {
		opt(&c)
	}

	return c
}

// filenames returns the names of the certificate and the key presented to API clients.
func (c certificate) filenames() (certificateFilename string, keyFilename string) {
	if c.clustered != nil && c.clustered() {
		return config.ClusterCertificateFilename, config.ClusterKeyFilename
	}

	return config.ServerCertificateFilename, config.ServerKeyFilename
}

func (c certificate) GetServerCertificate(_ context.Context) ([]byte, error) {
	certificateFilename, _ := c.filenames()

	certificatePEM, err := c.varRoot.ReadFile(certificateFilename)
	if err != nil {
		return nil, fmt.Errorf("Failed to read %q: %w", certificateFilename, err)
	}

	return certificatePEM, nil
}

func (c certificate) GetServerKeyPair(ctx context.Context) (certificatePEM []byte, keyPEM []byte, _ error) {
	certificatePEM, err := c.GetServerCertificate(ctx)
	if err != nil {
		return nil, nil, err
	}

	_, keyFilename := c.filenames()

	keyPEM, err = c.varRoot.ReadFile(keyFilename)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to read %q: %w", keyFilename, err)
	}

	return certificatePEM, keyPEM, nil
}

func (c certificate) UpdateServerKeyPair(_ context.Context, certificatePEM []byte, keyPEM []byte) error {
	certificateFilename, keyFilename := c.filenames()

	err := file.WriteFileAtomicIn(c.varRoot, certificateFilename, certificatePEM, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write %q: %w", certificateFilename, err)
	}

	err = file.WriteFileAtomicIn(c.varRoot, keyFilename, keyPEM, 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write %q: %w", keyFilename, err)
	}

	return nil
}
