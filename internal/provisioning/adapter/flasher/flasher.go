package flasher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/seed"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

const seedTarballStartPosition = 2148532224

type Flasher struct {
	mu sync.Mutex

	serverURL         string
	serverCertificate string
}

var _ provisioning.FlasherPort = &Flasher{}

func New(serverURL string, serverCertificate tls.Certificate) *Flasher {
	flasher := &Flasher{
		mu:        sync.Mutex{},
		serverURL: serverURL,
	}

	flasher.UpdateCertificate(serverCertificate)

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

func (f *Flasher) GenerateSeededImage(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs, file io.ReadCloser) (_ io.ReadCloser, _ error) {
	providerConfig, err := f.GetProviderConfig(ctx, id)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("Failed to create seed tarball: %w", err)
	}

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize gzip reader: %w", err)
	}

	return newInjectReader(newParentCloser(gzipReader, file), seedTarballStartPosition, tarball), nil
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

	if !isSelfSigned(cert) {
		f.serverCertificate = ""
		return
	}

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})

	f.serverCertificate = string(serverCert)
}

// isSelfSigned checks if the provided TLS certificate is self-signed.
// A certificate is considered self-signed if its subject and issuer are the same.
// If in doubt, it returns false.
func isSelfSigned(cert tls.Certificate) bool {
	if cert.Leaf == nil {
		return false
	}

	if cert.Leaf.Subject.String() == cert.Leaf.Issuer.String() {
		return true
	}

	return false
}

func (f *Flasher) UpdateServerURL(serverURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.serverURL = serverURL
}
