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

func (f *Flasher) GenerateSeededISO(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenSeedConfig, file io.ReadCloser) (_ io.ReadCloser, _ error) {
	f.mu.Lock()
	serverURL := f.serverURL
	serverCertificate := f.serverCertificate
	f.mu.Unlock()

	if serverURL == "" {
		return nil, errors.New(`Unabled to generate seeded ISO, server URL is not provided. Set "address" in "config.yml".`)
	}

	applications := make([]seed.Application, 0, len(seedConfig.Applications))
	for _, application := range seedConfig.Applications {
		applications = append(applications, seed.Application{Name: application})
	}

	// Create seed tarball.
	var target *seed.InstallTarget
	if seedConfig.InstallTarget.ID != "" {
		target = &seedConfig.InstallTarget
	}

	seedProvider := &seed.Provider{
		SystemProviderConfig: incusosapi.SystemProviderConfig{
			Name: "operations-center",
			Config: map[string]string{
				"server_url":   serverURL,
				"server_token": id.String(),
			},
		},
		Version: "1",
	}

	if serverCertificate != "" {
		seedProvider.Config["server_certificate"] = serverCertificate
	}

	tarball, err := createSeedTarball(
		&seed.Applications{
			Applications: applications,
			Version:      "1",
		},
		&seed.Incus{
			ApplyDefaults: false,
			Version:       "1",
		},
		&seed.Install{
			ForceInstall: true,
			ForceReboot:  false,
			Target:       target,
			Version:      "1",
		},
		&seed.Network{
			SystemNetworkConfig: seedConfig.Network,
			Version:             "1",
		},
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

func createSeedTarball(applicationSeed *seed.Applications, incusSeed *seed.Incus, installSeed *seed.Install, networkSeed *seed.Network, providerSeed *seed.Provider) (_ []byte, err error) {
	seedData := []struct {
		filename string
		data     any
	}{
		{
			filename: "application.yaml",
			data:     applicationSeed,
		},
		{
			filename: "incus.yaml",
			data:     incusSeed,
		},
		{
			filename: "install.yaml",
			data:     installSeed,
		},
		{
			filename: "network.yaml",
			data:     networkSeed,
		},
		{
			filename: "provider.yaml",
			data:     providerSeed,
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
