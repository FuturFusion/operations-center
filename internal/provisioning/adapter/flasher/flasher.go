package flasher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/seed"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/shared/api"
)

const seedTarballStartPosition = 2148532224

type Flasher struct {
	mu sync.Mutex

	serverURL         string
	serverCertificate string

	cache *imageCache
}

var _ provisioning.FlasherPort = &Flasher{}

type Option func(f *Flasher)

// WithCacheDir enables the seed image cache and stores the cached images in dir.
//
// Without it, generating and opening a seeded image report the cache as
// unavailable.
func WithCacheDir(dir string) Option {
	return func(f *Flasher) {
		f.cache = newImageCache(dir)
	}
}

func New(serverURL string, serverCertificate tls.Certificate, opts ...Option) *Flasher {
	flasher := &Flasher{
		mu:        sync.Mutex{},
		serverURL: serverURL,
	}

	flasher.UpdateCertificate(serverCertificate)

	for _, opt := range opts {
		opt(flasher)
	}

	return flasher
}

func (f *Flasher) GetProviderConfig(ctx context.Context, tokenID uuid.UUID) (*api.TokenProviderConfig, error) {
	f.mu.Lock()
	serverURL := f.serverURL
	serverCertificate := f.serverCertificate
	f.mu.Unlock()

	if serverURL == "" {
		return nil, errors.New(`Unabled to generate seeded image, server URL is not provided. Set "address" in "config.yml".`)
	}

	seedProvider := &api.TokenProviderConfig{
		SystemProviderConfig: incusosapi.SystemProviderConfig{
			Name: "operations-center",
			Config: map[string]string{
				"server_url":   serverURL,
				"server_token": tokenID.String(),
			},
		},
		Version: "1",
	}

	if serverCertificate != "" {
		seedProvider.Config["server_certificate"] = serverCertificate
	}

	return seedProvider, nil
}

// seedTarball returns the seed tarball for the given token and seed
// configuration together with the offset it has to be injected at in the
// uncompressed image.
func (f *Flasher) seedTarball(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs) (offset int64, payload []byte, _ error) {
	providerConfig, err := f.GetProviderConfig(ctx, id)
	if err != nil {
		return 0, nil, err
	}

	seedProvider := &seed.Provider{
		SystemProviderConfig: providerConfig.SystemProviderConfig,
		Version:              providerConfig.Version,
	}

	tarball, err := createSeedTarball(
		seedConfig,
		seedProvider,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("Failed to create seed tarball: %w", err)
	}

	return seedTarballStartPosition, tarball, nil
}

// GenerateCompressedSeededImage returns a forward-only stream of the seeded
// image, taking the image from the compressed file, to be handed out as a
// compressed file again.
func (f *Flasher) GenerateCompressedSeededImage(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs, file io.ReadCloser) (io.ReadCloser, error) {
	offset, tarball, err := f.seedTarball(ctx, id, seedConfig)
	if err != nil {
		return nil, err
	}

	return seededImage(file, offset, tarball)
}

// seededImage returns the image from the compressed file with the seed tarball
// injected at offset.
func seededImage(file io.ReadCloser, offset int64, tarball []byte) (io.ReadCloser, error) {
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize gzip reader: %w", err)
	}

	return newInjectReader(newParentCloser(gzipReader, file), offset, tarball), nil
}

func createSeedTarball(seedConfig provisioning.TokenImageSeedConfigs, providerSeed *seed.Provider) (_ []byte, err error) {
	seedData := []struct {
		filename string
		data     any
	}{
		{
			filename: "applications.yaml",
			data:     seedConfig.Applications,
		},
		{
			filename: "incus.yaml",
			data:     seedConfig.Incus,
		},
		{
			filename: "install.yaml",
			data:     seedConfig.Install,
		},
		{
			filename: "migration-manager.yaml",
			data:     seedConfig.MigrationManager,
		},
		{
			filename: "network.yaml",
			data:     seedConfig.Network,
		},
		{
			filename: "operations-center.yaml",
			data:     seedConfig.OperationsCenter,
		},
		{
			filename: "provider.yaml",
			data:     providerSeed,
		},
		{
			filename: "security.yaml",
			data:     seedConfig.Security,
		},
		{
			filename: "update.yaml",
			data:     seedConfig.Update,
		},
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer func() {
		closeErr := tw.Close()
		if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}()

	for _, data := range seedData {
		body, err := yaml.Marshal(data.data)
		if err != nil {
			return nil, err
		}

		hdr := &tar.Header{
			Name: data.filename,
			Mode: 0o600,
			Size: int64(len(body)),
		}

		err = tw.WriteHeader(hdr)
		if err != nil {
			return nil, err
		}

		_, err = tw.Write(body)
		if err != nil {
			return nil, err
		}
	}

	err = tw.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (f *Flasher) UpdateCertificate(cert tls.Certificate) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if isSystemCertPoolTrusted(cert) {
		f.serverCertificate = ""
		return
	}

	f.serverCertificate = certificate.EncodeToPEM(cert.Certificate[0])
}

func isSystemCertPoolTrusted(cert tls.Certificate) bool {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return false
	}

	return isCertChainTrusted(cert, roots)
}

// isCertChainTrusted reports, if the leaf certificate can be verified against
// roots, taking the intermediate certificates of the chain into account.
func isCertChainTrusted(cert tls.Certificate, roots *x509.CertPool) bool {
	if len(cert.Certificate) == 0 {
		return false
	}

	leaf := cert.Leaf
	if leaf == nil {
		var err error

		leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return false
		}
	}

	intermediates := x509.NewCertPool()
	for _, cert := range cert.Certificate[1:] {
		intermediate, err := x509.ParseCertificate(cert)
		if err != nil {
			return false
		}

		intermediates.AddCert(intermediate)
	}

	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	})

	return err == nil
}

func (f *Flasher) UpdateServerURL(serverURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.serverURL = serverURL
}
