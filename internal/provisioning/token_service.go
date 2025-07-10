package provisioning

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type tokenService struct {
	repo      TokenRepo
	updateSvc UpdateService
	serverURL string

	randomUUID func() (uuid.UUID, error)
}

var _ TokenService = &tokenService{}

type TokenServiceOption func(s *tokenService)

func NewTokenService(repo TokenRepo, updateSvc UpdateService, serverURL string, opts ...TokenServiceOption) tokenService {
	tokenSvc := tokenService{
		repo:       repo,
		updateSvc:  updateSvc,
		serverURL:  serverURL,
		randomUUID: uuid.NewRandom,
	}

	for _, opt := range opts {
		opt(&tokenSvc)
	}

	return tokenSvc
}

func (s tokenService) Create(ctx context.Context, newToken Token) (Token, error) {
	var err error
	newToken.UUID, err = s.randomUUID()
	if err != nil {
		return Token{}, err
	}

	err = newToken.Validate()
	if err != nil {
		return Token{}, err
	}

	newToken.ID, err = s.repo.Create(ctx, newToken)
	if err != nil {
		return Token{}, err
	}

	return newToken, nil
}

func (s tokenService) GetAll(ctx context.Context) (Tokens, error) {
	return s.repo.GetAll(ctx)
}

func (s tokenService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s tokenService) GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s tokenService) Update(ctx context.Context, newToken Token) error {
	err := newToken.Validate()
	if err != nil {
		return err
	}

	return s.repo.Update(ctx, newToken)
}

func (s tokenService) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteByUUID(ctx, id)
}

func (s tokenService) Consume(ctx context.Context, id uuid.UUID) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		token, err := s.repo.GetByUUID(ctx, id)
		if err != nil {
			return fmt.Errorf("Consume token: %w", err)
		}

		if token.UsesRemaining < 1 {
			return fmt.Errorf("Token exhausted")
		}

		if time.Now().After(token.ExpireAt) {
			return fmt.Errorf("Token expired")
		}

		token.UsesRemaining--

		err = s.repo.Update(ctx, *token)
		if err != nil {
			return fmt.Errorf("Update token: %w", err)
		}

		return nil
	})
}

func (s tokenService) GetPreSeedISO(ctx context.Context, id uuid.UUID, seedConfig TokenSeedConfig) (_ io.ReadCloser, _ int, err error) {
	// TODO: Allow filters?
	updates, err := s.updateSvc.GetAll(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to get updates: %w", err)
	}

	if len(updates) == 0 {
		return nil, 0, fmt.Errorf("Failed to get updates: No updates found")
	}

	// Update service does return the updates ordered by version in descending order.
	latestUpdate := updates[0]

	updateFiles, err := s.updateSvc.GetUpdateAllFiles(ctx, latestUpdate.UUID)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to get files for update %q: %w", latestUpdate.UUID, err)
	}

	var filename string
	for _, file := range updateFiles {
		// TODO: filter for the correct architecture.
		if file.Type == api.UpdateFileTypeImageISO {
			filename = file.Filename
			break
		}
	}

	if filename == "" {
		return nil, 0, fmt.Errorf("Failed to find ISO file for latest update %q", latestUpdate.UUID)
	}

	filereader, _, err := s.updateSvc.GetUpdateFileByFilename(ctx, latestUpdate.UUID, filename)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to get file %q form latest update %q: %w", filename, latestUpdate.UUID, err)
	}

	decompressedFilereader, err := gzip.NewReader(filereader)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to read gzip content of file %q from latest update %q: %w", filename, latestUpdate.UUID, err)
	}

	preSeedFile, err := os.CreateTemp("", "operations-center-pre-seed-*.iso")
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to create temporary pre-seed file: %w", err)
	}

	defer func() {
		closeErr := preSeedFile.Close()
		if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, fmt.Errorf("Failed to close temporary pre-seed file: %w", closeErr))
		}
	}()

	// Read from the decompressor in chunks to avoid excessive memory consumption.
	for {
		_, err = io.CopyN(preSeedFile, decompressedFilereader, 4*1024*1024)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, 0, fmt.Errorf("Failed to copy file %q from latest update %q to temporary pre-seed file: %w", filename, latestUpdate.UUID, err)
		}
	}

	err = preSeedFile.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to close temporary pre-seed file: %w", err)
	}

	applications := make([]Application, 0, len(seedConfig.Applications))
	for _, application := range seedConfig.Applications {
		applications = append(applications, Application{Name: application})
	}

	// Create seed tarball.
	tarball, err := createSeedTarball(
		&Applications{
			Applications: applications,
			Version:      "",
		},
		&IncusConfig{
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
		&InstallSeed{
			ForceInstall: true,
			ForceReboot:  true,
			// Target: &InstallSeedTarget{
			// 	ID: "",
			// },
			Version: "",
		},
		&NetworkSeed{
			SystemNetworkConfig: seedConfig.Network,
			Version:             "",
		},
		&ProviderSeed{
			SystemProviderConfig: incusosapi.SystemProviderConfig{
				Name: "images",
				Config: map[string]string{
					"server_url":   s.serverURL,
					"server_token": id.String(),
				},
			},
			Version: "",
		},
	)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to create seed tarball: %w", err)
	}

	// Copy seed tarball to fixed location in the ISO file.
	err = injectSeedIntoImage(preSeedFile.Name(), tarball)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to inject seed into image: %w", err)
	}

	// Return reader to file to the user.
	f, err := os.Open(preSeedFile.Name())
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to open temporary seed file: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to stat temporary seed file: %w", err)
	}

	// TODO: cleanup temporary file, implement delete on close?

	return f, int(fi.Size()), nil
}

func createSeedTarball(applicationSeed *Applications, incusSeed *IncusConfig, installSeed *InstallSeed, networkSeed *NetworkSeed, providerSeed *ProviderSeed) (_ []byte, err error) {
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

	for _, seed := range seedData {
		body, err := yaml.Marshal(seed.data)
		if err != nil {
			return nil, err
		}

		hdr := &tar.Header{
			Name: seed.filename,
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

func injectSeedIntoImage(imageFilename string, data []byte) error {
	tgt, err := os.OpenFile(imageFilename, os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		closeErr := tgt.Close()
		if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}()

	// TODO: move 2148532224 to a constant
	numBytes, err := tgt.WriteAt(data, 2148532224)
	if err != nil {
		return err
	}

	if numBytes != len(data) {
		return fmt.Errorf("Failed to write seed tar archive into image: copied %d of %d bytes", numBytes, len(data))
	}

	return nil
}
