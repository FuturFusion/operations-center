package flasher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
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

func (f *Flasher) GenerateSeededImage(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs, file io.ReadCloser) (_ io.ReadCloser, size int, _ error) {
	providerConfig, err := f.GetProviderConfig(ctx, id)
	if err != nil {
		return nil, -1, err
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
		return nil, -1, fmt.Errorf("Failed to create seed tarball: %w", err)
	}

	size, err = uncompressedSize(file, seedTarballStartPosition+len(tarball))
	if err != nil {
		return nil, -1, fmt.Errorf("Failed to determine size of the image: %w", err)
	}

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, -1, fmt.Errorf("Failed to initialize gzip reader: %w", err)
	}

	return newInjectReader(newParentCloser(gzipReader, file), seedTarballStartPosition, tarball), size, nil
}

// uncompressedSize returns the size of the content of the gzip stream in
// bytes, taken from the ISIZE field in its footer, and leaves the reader at
// the position it was called with.
//
// It reports -1 if the size cannot be determined.
func uncompressedSize(r io.Reader, minSize int) (int, error) {
	seeker, ok := r.(io.Seeker)
	if !ok {
		return -1, nil
	}

	position, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return -1, err
	}

	_, err = seeker.Seek(-4, io.SeekEnd)
	if err != nil {
		return -1, err
	}

	var footer [4]byte

	_, readErr := io.ReadFull(r, footer[:])

	_, err = seeker.Seek(position, io.SeekStart)
	if err != nil {
		return -1, err
	}

	if readErr != nil {
		return -1, readErr
	}

	size := int64(binary.LittleEndian.Uint32(footer[:]))
	if size < int64(minSize) {
		return -1, nil
	}

	return int(size), nil
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

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate[0],
	})

	f.serverCertificate = string(serverCert)
}

func isSystemCertPoolTrusted(cert tls.Certificate) bool {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return false
	}

	opts := x509.VerifyOptions{Roots: roots}
	_, err = cert.Leaf.Verify(opts)

	return err == nil
}

func (f *Flasher) UpdateServerURL(serverURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.serverURL = serverURL
}
