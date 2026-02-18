package provisioning

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type tokenService struct {
	repo       TokenRepo
	updateSvc  UpdateService
	channelSvc ChannelService
	flasher    FlasherPort

	randomUUID func() (uuid.UUID, error)

	imagesMu sync.Mutex
	images   map[uuid.UUID]imageRecord
}

var _ TokenService = &tokenService{}

type TokenServiceOption func(s *tokenService)

func NewTokenService(repo TokenRepo, updateSvc UpdateService, channelSvc ChannelService, flasher FlasherPort, opts ...TokenServiceOption) *tokenService {
	tokenSvc := &tokenService{
		repo:       repo,
		updateSvc:  updateSvc,
		channelSvc: channelSvc,
		flasher:    flasher,
		randomUUID: uuid.NewRandom,
		imagesMu:   sync.Mutex{},
		images:     map[uuid.UUID]imageRecord{},
	}

	for _, opt := range opts {
		opt(tokenSvc)
	}

	return tokenSvc
}

func (s *tokenService) Create(ctx context.Context, newToken Token) (Token, error) {
	var err error
	newToken.UUID, err = s.randomUUID()
	if err != nil {
		return Token{}, fmt.Errorf("Failed to generate UUID for new token: %w", err)
	}

	if newToken.Channel == "" {
		newToken.Channel = config.GetUpdates().ServerDefaultChannel
	}

	err = newToken.Validate()
	if err != nil {
		return Token{}, fmt.Errorf("Validation failed for new token: %w", err)
	}

	newToken.ID, err = s.repo.Create(ctx, newToken)
	if err != nil {
		return Token{}, fmt.Errorf("Failed to create token: %w", err)
	}

	return newToken, nil
}

func (s *tokenService) GetAll(ctx context.Context) (Tokens, error) {
	return s.repo.GetAll(ctx)
}

func (s *tokenService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s *tokenService) GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s *tokenService) Update(ctx context.Context, newToken Token) error {
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
			return fmt.Errorf("Consume token: %w", err)
		}

		if token.UsesRemaining < 1 {
			return fmt.Errorf("Token exhausted: %w", domain.ErrOperationNotPermitted)
		}

		if time.Now().After(token.ExpireAt) {
			return fmt.Errorf("Token expired: %w", domain.ErrOperationNotPermitted)
		}

		token.UsesRemaining--

		err = s.repo.Update(ctx, *token)
		if err != nil {
			return fmt.Errorf("Update token: %w", err)
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
	SeedConfig   TokenImageSeedConfigs
	CreatedAt    time.Time
}

func (s *tokenService) PreparePreSeededImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture images.UpdateFileArchitecture, seedConfig TokenImageSeedConfigs) (uuid.UUID, error) {
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

func (s *tokenService) CreateTokenSeed(ctx context.Context, tokenSeed TokenSeed) (TokenSeed, error) {
	err := tokenSeed.Validate()
	if err != nil {
		return TokenSeed{}, fmt.Errorf("Validate token seed: %w", err)
	}

	tokenSeed.ID, err = s.repo.CreateTokenSeed(ctx, tokenSeed)
	if err != nil {
		return TokenSeed{}, err
	}

	return tokenSeed, nil
}

func (s *tokenService) GetTokenSeedAll(ctx context.Context, id uuid.UUID) (TokenSeeds, error) {
	tokenSeeds, err := s.repo.GetTokenSeedAll(ctx, id)
	if err != nil {
		return nil, err
	}

	return tokenSeeds, nil
}

func (s *tokenService) GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error) {
	return s.repo.GetTokenSeedAllNames(ctx, id)
}

func (s *tokenService) GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*TokenSeed, error) {
	tokenSeedConfig, err := s.repo.GetTokenSeedByName(ctx, id, name)
	if err != nil {
		return nil, err
	}

	return tokenSeedConfig, nil
}

func (s *tokenService) UpdateTokenSeed(ctx context.Context, tokenSeed TokenSeed) error {
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

func (s *tokenService) GetTokenImageFromTokenSeed(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error) {
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

func (s *tokenService) getPreSeedImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string, seeds TokenImageSeedConfigs) (_ io.ReadCloser, err error) {
	if channel == "" {
		channel = config.GetUpdates().UpdatesDefaultChannel
	}

	updates, err := s.updateSvc.GetAllWithFilter(ctx, UpdateFilter{
		Status:  ptr.To(api.UpdateStatusReady),
		Channel: ptr.To(channel),
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to get updates: %w", err)
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("Failed to get updates: No ready updates found in channel %q: %w", channel, domain.ErrNotFound)
	}

	// Update service does return the updates ordered by version in descending order.
	latestUpdate := updates[0]

	updateFiles, err := s.updateSvc.GetUpdateAllFiles(ctx, latestUpdate.UUID)
	if err != nil {
		return nil, fmt.Errorf("Failed to get files for update %q: %w", latestUpdate.UUID.String(), err)
	}

	var filename string
	for _, file := range updateFiles {
		if file.Type == imageType.UpdateFileType() && file.Architecture == architecture {
			filename = file.Filename
			break
		}
	}

	if filename == "" {
		return nil, fmt.Errorf("Failed to find image file of type %q for architecture %q in latest update %q", imageType, architecture, latestUpdate.UUID.String())
	}

	filereader, _, err := s.updateSvc.GetUpdateFileByFilename(ctx, latestUpdate.UUID, filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to get file %q form latest update %q: %w", filename, latestUpdate.UUID.String(), err)
	}

	file, ok := filereader.(*os.File)
	if !ok {
		return nil, fmt.Errorf("Latest update %q is not a file", filename)
	}

	// Apply defaults to seeds.
	if len(seeds.Applications) == 0 {
		seeds.Applications = map[string]any{
			"version": "1",
			"applications": []any{
				map[string]any{
					"name": "incus",
				},
			},
		}
	}

	// Enforce incus pre seeds applicable for use with Operations Center.
	if seeds.Incus == nil {
		seeds.Incus = map[string]any{}
	}

	seeds.Incus["apply_defaults"] = false
	seeds.Incus["version"] = "1"

	// Enforce update control through Operations Center.
	if seeds.Update == nil {
		seeds.Update = map[string]any{}
	}

	seeds.Update["version"] = "1"
	seeds.Update["auto_reboot"] = false
	seeds.Update["check_frequency"] = "never"
	seeds.Update["channel"], err = s.ensureChannelName(ctx, seeds.Update, channel)
	if err != nil {
		return nil, fmt.Errorf("Failed to validate update channel from seed config: %w", err)
	}

	rc, err := s.flasher.GenerateSeededImage(ctx, id, seeds, file)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate seeded image: %w", err)
	}

	return rc, nil
}

func (s *tokenService) ensureChannelName(ctx context.Context, update map[string]any, channel string) (string, error) {
	anyChannel, ok := update["channel"]
	if !ok {
		anyChannel = channel
	}

	channel, ok = anyChannel.(string)
	if !ok {
		return "", domain.NewValidationErrf(`Invalid type for update channel, "string" expected`)
	}

	_, err := s.channelSvc.GetByName(ctx, channel)
	if err != nil {
		return "", err
	}

	return channel, nil
}
