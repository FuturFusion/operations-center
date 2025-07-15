package flasher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/seed"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

const seedTarballStartPosition = 2148532224

type flasher struct {
	serverURL         string
	serverCertificate string
}

var _ provisioning.FlasherPort = flasher{}

func New(serverURL string, serverCertificate string) provisioning.FlasherPort {
	return flasher{
		serverURL:         serverURL,
		serverCertificate: serverCertificate, // TODO: return as part of the seed, does require https://github.com/lxc/incus-os/issues/208.
	}
}

func (f flasher) GenerateSeededISO(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenSeedConfig, file io.ReadCloser) (_ io.ReadCloser, _ error) {
	applications := make([]seed.Application, 0, len(seedConfig.Applications))
	for _, application := range seedConfig.Applications {
		applications = append(applications, seed.Application{Name: application})
	}

	// Create seed tarball.
	tarball, err := createSeedTarball(
		&seed.Applications{
			Applications: applications,
			Version:      "",
		},
		&seed.Incus{
			// ApplyDefaults: true,
			// Preseed: &incusapi.InitPreseed{
			// 	Server: incusapi.InitLocalPreseed{
			// 		ServerPut: incusapi.ServerPut{
			// 			Config: map[string]string{},
			// 		},
			// 		Certificates: []incusapi.CertificatesPost{},
			// 	},
			// },
			Version: "",
		},
		&seed.Install{
			ForceInstall: true,
			ForceReboot:  true,
			// Target: &InstallSeedTarget{
			// 	ID: "",
			// },
			Version: "",
		},
		&seed.Network{
			SystemNetworkConfig: seedConfig.Network,
			Version:             "",
		},
		&seed.Provider{
			SystemProviderConfig: incusosapi.SystemProviderConfig{
				Name: "images",
				Config: map[string]string{
					"server_url":   f.serverURL,
					"server_token": id.String(),
				},
			},
			Version: "",
		},
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
