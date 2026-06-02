package token

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type tokenService struct {
	repo       provisioning.TokenRepo
	updateSvc  provisioning.UpdateService
	channelSvc provisioning.ChannelService
	flasher    provisioning.FlasherPort
	client     provisioning.TokenClientPort

	randomUUID func() (uuid.UUID, error)

	imagesMu sync.Mutex
	images   map[uuid.UUID]imageRecord
}

var _ provisioning.TokenService = &tokenService{}

type Option func(s *tokenService)

func New(repo provisioning.TokenRepo, updateSvc provisioning.UpdateService, channelSvc provisioning.ChannelService, flasher provisioning.FlasherPort, client provisioning.TokenClientPort, opts ...Option) *tokenService {
	tokenSvc := &tokenService{
		repo:       repo,
		updateSvc:  updateSvc,
		channelSvc: channelSvc,
		flasher:    flasher,
		client:     client,
		randomUUID: uuid.NewRandom,
		imagesMu:   sync.Mutex{},
		images:     map[uuid.UUID]imageRecord{},
	}

	for _, opt := range opts {
		opt(tokenSvc)
	}

	return tokenSvc
}

func (s *tokenService) Create(ctx context.Context, newToken provisioning.Token) (provisioning.Token, error) {
	var err error
	newToken.UUID, err = s.randomUUID()
	if err != nil {
		return provisioning.Token{}, fmt.Errorf("Failed to generate UUID for new token: %w", err)
	}

	if newToken.Channel == "" {
		newToken.Channel = config.GetUpdates().ServerDefaultChannel
	}

	err = newToken.Validate()
	if err != nil {
		return provisioning.Token{}, fmt.Errorf("Validation failed for new token: %w", err)
	}

	newToken.ID, err = s.repo.Create(ctx, newToken)
	if err != nil {
		return provisioning.Token{}, fmt.Errorf("Failed to create token: %w", err)
	}

	return newToken, nil
}

func (s *tokenService) GetAll(ctx context.Context) (provisioning.Tokens, error) {
	return s.repo.GetAll(ctx)
}

func (s *tokenService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s *tokenService) GetByUUID(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s *tokenService) Update(ctx context.Context, newToken provisioning.Token) error {
	err := newToken.Validate()
	if err != nil {
		return fmt.Errorf("Validation failed for token update: %w", err)
	}

	return s.repo.Update(ctx, newToken)
}

func (s *tokenService) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteByUUID(ctx, id)
}

func (s *tokenService) Consume(ctx context.Context, id uuid.UUID) (channel string, _ error) {
	err := transaction.Do(ctx, func(ctx context.Context) error {
		token, err := s.repo.GetByUUID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("Consume token: %w", domain.ErrNotAuthorized)
			}

			return fmt.Errorf("Consume token: %w", err)
		}

		if token.UsesRemaining < 1 {
			return fmt.Errorf("Token exhausted: %w", domain.ErrOperationNotPermitted)
		}

		if time.Now().After(token.ExpireAt) {
			return fmt.Errorf("Token expired: %w", domain.ErrOperationNotPermitted)
		}

		token.UsesRemaining--

		if token.AutoRemove && token.UsesRemaining == 0 {
			err = s.repo.DeleteByUUID(ctx, id)
		} else {
			err = s.repo.Update(ctx, *token)
		}

		if err != nil {
			return fmt.Errorf("Update token %s: %w", id.String(), err)
		}

		channel = token.Channel

		return nil
	})
	if err != nil {
		return "", err
	}

	return channel, nil
}

type imageRecord struct {
	TokenID      uuid.UUID
	ImageType    api.ImageType
	Architecture images.UpdateFileArchitecture
	Channel      string
	SeedConfig   provisioning.TokenImageSeedConfigs
	CreatedAt    time.Time
}

func (s *tokenService) PreparePreSeededImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture images.UpdateFileArchitecture, seedConfig provisioning.TokenImageSeedConfigs) (uuid.UUID, error) {
	s.imagesMu.Lock()
	defer s.imagesMu.Unlock()

	// Remove image records older than 5 minutes.
	for imageUUID, image := range s.images {
		if time.Since(image.CreatedAt) > 5*time.Minute {
			delete(s.images, imageUUID)
		}
	}

	if !imageType.IsValid() {
		return uuid.Nil, domain.NewValidationErrf("Invalid image type")
	}

	_, ok := images.UpdateFileArchitectures[architecture]
	if !ok {
		return uuid.Nil, domain.NewValidationErrf("Invalid architecture")
	}

	token, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("Unable to get token %s: %w", id.String(), err)
	}

	imageUUID := uuid.New()

	s.images[imageUUID] = imageRecord{
		TokenID:      id,
		ImageType:    imageType,
		Architecture: architecture,
		Channel:      token.Channel,
		SeedConfig:   seedConfig,
		CreatedAt:    time.Now(),
	}

	return imageUUID, nil
}

func (s *tokenService) GetPreSeededImage(ctx context.Context, id uuid.UUID, imageUUID uuid.UUID) (_ io.ReadCloser, filename string, _ error) {
	s.imagesMu.Lock()
	// Remove image records older than 5 minutes.
	for imageUUID, image := range s.images {
		if time.Since(image.CreatedAt) > 5*time.Minute {
			delete(s.images, imageUUID)
		}
	}

	image, ok := s.images[imageUUID]
	s.imagesMu.Unlock()
	if !ok {
		return nil, "", fmt.Errorf("Failed to find image configuration for uuid %q: %w", imageUUID.String(), domain.ErrNotFound)
	}

	if image.TokenID != id {
		return nil, "", fmt.Errorf("Image configuration %q does not match token id %q: %w", imageUUID.String(), id.String(), domain.ErrConstraintViolation)
	}

	_, err := s.repo.GetByUUID(ctx, image.TokenID)
	if err != nil {
		return nil, "", fmt.Errorf("Unable to get token %s: %w", image.TokenID.String(), err)
	}

	rc, err := s.getPreSeedImage(ctx, image.TokenID, image.ImageType, image.Architecture, image.Channel, image.SeedConfig)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to get pre seed image stream: %w", err)
	}

	s.imagesMu.Lock()
	delete(s.images, imageUUID)
	s.imagesMu.Unlock()

	return rc, fmt.Sprintf("pre-seed-%s%s", image.TokenID.String(), image.ImageType.FileExt()), nil
}

func (s *tokenService) GetTokenProviderConfig(ctx context.Context, id uuid.UUID) (*api.TokenProviderConfig, error) {
	seedProvider, err := s.flasher.GetProviderConfig(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get provider config for token %q: %w", id.String(), err)
	}

	return seedProvider, nil
}

func (s *tokenService) CreateTokenSeed(ctx context.Context, tokenSeed provisioning.TokenSeed) (provisioning.TokenSeed, error) {
	err := tokenSeed.Validate()
	if err != nil {
		return provisioning.TokenSeed{}, fmt.Errorf("Validate token seed: %w", err)
	}

	tokenSeed.ID, err = s.repo.CreateTokenSeed(ctx, tokenSeed)
	if err != nil {
		return provisioning.TokenSeed{}, err
	}

	return tokenSeed, nil
}

func (s *tokenService) GetTokenSeedAll(ctx context.Context, id uuid.UUID) (provisioning.TokenSeeds, error) {
	tokenSeeds, err := s.repo.GetTokenSeedAll(ctx, id)
	if err != nil {
		return nil, err
	}

	return tokenSeeds, nil
}

func (s *tokenService) GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error) {
	return s.repo.GetTokenSeedAllNames(ctx, id)
}

func (s *tokenService) GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
	tokenSeedConfig, err := s.repo.GetTokenSeedByName(ctx, id, name)
	if err != nil {
		return nil, err
	}

	return tokenSeedConfig, nil
}

func (s *tokenService) UpdateTokenSeed(ctx context.Context, tokenSeed provisioning.TokenSeed) error {
	err := tokenSeed.Validate()
	if err != nil {
		return fmt.Errorf("Validate token seed: %w", err)
	}

	err = s.repo.UpdateTokenSeed(ctx, tokenSeed)
	if err != nil {
		return err
	}

	return nil
}

func (s *tokenService) DeleteTokenSeedByName(ctx context.Context, id uuid.UUID, name string) error {
	err := s.repo.DeleteTokenSeedByName(ctx, id, name)
	if err != nil {
		return err
	}

	return nil
}

// GetCompressedTokenImageFromTokenSeed returns the pre-seeded image for a
// token seed as a forward-only stream, to be handed out as a compressed file.
func (s *tokenService) GetCompressedTokenImageFromTokenSeed(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error) {
	if !imageType.IsValid() {
		return nil, domain.NewValidationErrf("Invalid image type")
	}

	_, ok := images.UpdateFileArchitectures[architecture]
	if !ok {
		return nil, domain.NewValidationErrf("Invalid architecture")
	}

	_, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get token %q: %w", id.String(), err)
	}

	tokenSeed, err := s.repo.GetTokenSeedByName(ctx, id, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get token seed %q for token %q: %w", name, id.String(), err)
	}

	return s.getPreSeedImage(ctx, id, imageType, architecture, channel, tokenSeed.Seeds)
}

// GetSeekableTokenImageFromTokenSeed returns the pre-seeded image for a token
// seed as a seekable stream, so that arbitrary byte ranges of it can be served.
//
// It resolves and, if need be, generates the image, so it answers a request
// naming a token seed. A request naming a prepared image is answered by
// GetPreparedTokenSeedImage instead, which takes no lookup at all.
func (s *tokenService) GetSeekableTokenImageFromTokenSeed(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (*provisioning.TokenImage, error) {
	tokenSeed, err := s.tokenSeedForImage(ctx, id, name, imageType, architecture)
	if err != nil {
		return nil, err
	}

	cacheID := provisioning.SeedImageCacheID(id, name, imageType, architecture, channel)

	content, size, modTime, err := s.generateSeedImage(ctx, cacheID, id, *tokenSeed, imageType, architecture, channel)
	if err != nil {
		return nil, err
	}

	return &provisioning.TokenImage{
		Content:  content,
		Size:     size,
		ModTime:  modTime,
		Filename: seedImageFilename(name, imageType),
	}, nil
}

// ResolveTokenSeedImageID returns the ID identifying the pre-seeded image a
// token seed currently resolves to, without generating it.
//
// It establishes the address the image is served under, so it insists on the
// token seed being public.
func (s *tokenService) ResolveTokenSeedImageID(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
	tokenSeed, err := s.publicTokenSeedForImage(ctx, id, name, imageType, architecture)
	if err != nil {
		return "", err
	}

	fingerprint, _, _, seeds, err := s.resolveSeedImage(ctx, imageType, architecture, channel, tokenSeed.Seeds)
	if err != nil {
		return "", err
	}

	return s.flasher.SeedImageFingerprintID(ctx, fingerprint, id, seeds)
}

// PrepareTokenSeedImage generates the pre-seeded image for a public token seed
// and stores it, so that a request naming it can be answered without anything
// having to be looked up.
func (s *tokenService) PrepareTokenSeedImage(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) error {
	tokenSeed, err := s.publicTokenSeedForImage(ctx, id, name, imageType, architecture)
	if err != nil {
		return err
	}

	cacheID := provisioning.SeedImageCacheID(id, name, imageType, architecture, channel)

	content, _, _, err := s.generateSeedImage(ctx, cacheID, id, *tokenSeed, imageType, architecture, channel)
	if err != nil {
		return err
	}

	return content.Close()
}

// GetPreparedTokenSeedImage returns the pre-seeded image addressed by
// fingerprintID as a seekable stream.
func (s *tokenService) GetPreparedTokenSeedImage(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, fingerprintID string) (*provisioning.TokenImage, error) {
	err := validateSeedImageRequest(imageType, architecture)
	if err != nil {
		return nil, err
	}

	cacheID := provisioning.SeedImageCacheID(id, name, imageType, architecture, channel)

	content, size, modTime, err := s.flasher.OpenSeededImage(ctx, cacheID, fingerprintID)
	if err != nil {
		return nil, err
	}

	return &provisioning.TokenImage{
		Content:  content,
		Size:     size,
		ModTime:  modTime,
		Filename: seedImageFilename(name, imageType),
	}, nil
}

func validateSeedImageRequest(imageType api.ImageType, architecture images.UpdateFileArchitecture) error {
	if !imageType.IsValid() {
		return domain.NewValidationErrf("Invalid image type")
	}

	_, ok := images.UpdateFileArchitectures[architecture]
	if !ok {
		return domain.NewValidationErrf("Invalid architecture")
	}

	return nil
}

func seedImageFilename(name string, imageType api.ImageType) string {
	return fmt.Sprintf("pre-seed-%s%s", name, imageType.FileExt())
}

// tokenSeedForImage returns the token seed a pre-seeded image is asked for,
// after checking the image is one which can be built at all.
func (s *tokenService) tokenSeedForImage(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture) (*provisioning.TokenSeed, error) {
	err := validateSeedImageRequest(imageType, architecture)
	if err != nil {
		return nil, err
	}

	_, err = s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get token %q: %w", id.String(), err)
	}

	tokenSeed, err := s.repo.GetTokenSeedByName(ctx, id, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get token seed %q for token %q: %w", name, id.String(), err)
	}

	return tokenSeed, nil
}

// publicTokenSeedForImage returns the token seed only if its pre-seeded image
// may be fetched without authorization.
func (s *tokenService) publicTokenSeedForImage(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture) (*provisioning.TokenSeed, error) {
	tokenSeed, err := s.tokenSeedForImage(ctx, id, name, imageType, architecture)
	if err != nil {
		return nil, err
	}

	if !tokenSeed.Public {
		return nil, fmt.Errorf("Token seed %q is not public, so its pre-seeded image is not served under an address of its own: %w", name, domain.ErrOperationNotPermitted)
	}

	return tokenSeed, nil
}

// resolveSeedImage resolves what the pre-seeded image for a token seed is
// generated from: the fingerprint identifying those inputs, the file of the
// update it is built from, and the seed configuration going into it.
func (s *tokenService) resolveSeedImage(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, seeds provisioning.TokenImageSeedConfigs) (fingerprint string, updateUUID uuid.UUID, filename string, _ provisioning.TokenImageSeedConfigs, _ error) {
	updateUUID, filename, seeds, err := s.resolvePreSeedImage(ctx, imageType, architecture, channel, seeds)
	if err != nil {
		return "", uuid.Nil, "", seeds, err
	}

	fingerprint = strings.Join([]string{updateUUID.String(), filename}, "\x00")

	return fingerprint, updateUUID, filename, seeds, nil
}

// generateSeedImage resolves and generates the pre-seeded image for a token
// seed, unless it is stored already, and returns it.
//
// An image for a token seed, which is not public, is kept where a request
// naming it cannot reach it.
func (s *tokenService) generateSeedImage(ctx context.Context, cacheID string, id uuid.UUID, tokenSeed provisioning.TokenSeed, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (_ io.ReadSeekCloser, size int64, modTime time.Time, _ error) {
	fingerprint, updateUUID, filename, seeds, err := s.resolveSeedImage(ctx, imageType, architecture, channel, tokenSeed.Seeds)
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	filereader, _, err := s.updateSvc.GetUpdateFileByFilename(ctx, updateUUID, filename)
	if err != nil {
		return nil, 0, time.Time{}, fmt.Errorf("Failed to get file %q form latest update %q: %w", filename, updateUUID.String(), err)
	}

	return s.flasher.GenerateSeededImage(ctx, cacheID, fingerprint, id, seeds, tokenSeed.Public, filereader)
}

func (s *tokenService) getPreSeedImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, seeds provisioning.TokenImageSeedConfigs) (_ io.ReadCloser, err error) {
	updateUUID, filename, seeds, err := s.resolvePreSeedImage(ctx, imageType, architecture, channel, seeds)
	if err != nil {
		return nil, err
	}

	filereader, _, err := s.updateSvc.GetUpdateFileByFilename(ctx, updateUUID, filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to get file %q form latest update %q: %w", filename, updateUUID.String(), err)
	}

	rc, err := s.flasher.GenerateCompressedSeededImage(ctx, id, seeds, filereader)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("Failed to generate seeded image: %w", err), filereader.Close())
	}

	return rc, nil
}

func (s *tokenService) resolvePreSeedImage(ctx context.Context, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, seeds provisioning.TokenImageSeedConfigs) (updateUUID uuid.UUID, filename string, _ provisioning.TokenImageSeedConfigs, err error) {
	if channel == "" {
		channel = config.GetUpdates().UpdatesDefaultChannel
	}

	updates, err := s.updateSvc.GetAllWithFilter(ctx, provisioning.UpdateFilter{
		Status:  new(api.UpdateStatusReady),
		Channel: new(channel),
	})
	if err != nil {
		return uuid.Nil, "", seeds, fmt.Errorf("Failed to get updates: %w", err)
	}

	if len(updates) == 0 {
		return uuid.Nil, "", seeds, fmt.Errorf("Failed to get updates: No ready updates found in channel %q: %w", channel, domain.ErrNotFound)
	}

	// Update service does return the updates ordered by version in descending order.
	latestUpdate := updates[0]

	updateFiles, err := s.updateSvc.GetUpdateAllFiles(ctx, latestUpdate.UUID)
	if err != nil {
		return uuid.Nil, "", seeds, fmt.Errorf("Failed to get files for update %q: %w", latestUpdate.UUID.String(), err)
	}

	securityConfig, err := s.client.GetSecurityConfig(ctx, provisioning.ServerSelf)
	if err != nil {
		return uuid.Nil, "", seeds, fmt.Errorf("Failed to get operations-center's security config: %w", err)
	}

	for _, file := range updateFiles {
		if file.Type == imageType.UpdateFileType() && file.Architecture == architecture {
			filename = file.Filename
			break
		}
	}

	if filename == "" {
		return uuid.Nil, "", seeds, fmt.Errorf("Failed to find image file of type %q for architecture %q in latest update %q: %w", imageType, architecture, latestUpdate.UUID.String(), domain.ErrNotFound)
	}

	// Apply defaults to seeds.
	if seeds.Applications.Version == "" {
		seeds.Applications = api.SeedApplications{
			Version: "1",
			Applications: []api.SeedApplication{
				{
					Name: "incus",
				},
			},
		}
	}

	seeds.Incus = api.SeedIncus{
		Version:       "1",
		ApplyDefaults: false,
	}

	seeds.Security.Version = "1"
	seeds.Security.CustomCACerts = securityConfig.Config.CustomCACerts

	_, err = s.channelSvc.GetByName(ctx, channel)
	if err != nil {
		return uuid.Nil, "", seeds, fmt.Errorf("Failed to validate update channel %q: %w", channel, err)
	}

	seeds.Update = api.SeedUpdate{
		Version: "1",
		SystemUpdateConfig: api.SeedUpdateConfig{
			AutoReboot:     false,
			Channel:        channel,
			CheckFrequency: "never",
		},
	}

	return latestUpdate.UUID, filename, seeds, nil
}
