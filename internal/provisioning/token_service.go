package provisioning

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type tokenService struct {
	repo      TokenRepo
	updateSvc UpdateService
	flasher   FlasherPort

	randomUUID func() (uuid.UUID, error)
}

var _ TokenService = &tokenService{}

type TokenServiceOption func(s *tokenService)

func NewTokenService(repo TokenRepo, updateSvc UpdateService, flasher FlasherPort, opts ...TokenServiceOption) tokenService {
	tokenSvc := tokenService{
		repo:       repo,
		updateSvc:  updateSvc,
		flasher:    flasher,
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
		return Token{}, fmt.Errorf("Failed to generate UUID for new token: %w", err)
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
		return fmt.Errorf("Validation failed for token update: %w", err)
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

		return nil
	})
}

func (s tokenService) GetPreSeedImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture api.Architecture, seeds TokenImageSeedConfigs) (_ io.ReadCloser, err error) {
	if !imageType.IsValid() {
		return nil, domain.NewValidationErrf("Invalid image type")
	}

	if !architecture.IsValid() {
		return nil, domain.NewValidationErrf("Invalid architecture")
	}

	_, err = s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Unable to get token %s: %w", id.String(), err)
	}

	return s.getPreSeedImage(ctx, id, imageType, architecture, seeds)
}

func (s tokenService) CreateTokenSeed(ctx context.Context, tokenSeed TokenSeed) (TokenSeed, error) {
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

func (s tokenService) GetTokenSeedAll(ctx context.Context, id uuid.UUID) (TokenSeeds, error) {
	tokenSeeds, err := s.repo.GetTokenSeedAll(ctx, id)
	if err != nil {
		return nil, err
	}

	return tokenSeeds, nil
}

func (s tokenService) GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error) {
	return s.repo.GetTokenSeedAllNames(ctx, id)
}

func (s tokenService) GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*TokenSeed, error) {
	tokenSeedConfig, err := s.repo.GetTokenSeedByName(ctx, id, name)
	if err != nil {
		return nil, err
	}

	return tokenSeedConfig, nil
}

func (s tokenService) UpdateTokenSeed(ctx context.Context, tokenSeed TokenSeed) error {
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

func (s tokenService) DeleteTokenSeedByName(ctx context.Context, id uuid.UUID, name string) error {
	err := s.repo.DeleteTokenSeedByName(ctx, id, name)
	if err != nil {
		return err
	}

	return nil
}

func (s tokenService) GetTokenImageFromTokenSeed(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture api.Architecture) (io.ReadCloser, error) {
	if !imageType.IsValid() {
		return nil, domain.NewValidationErrf("Invalid image type")
	}

	if !architecture.IsValid() {
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

	return s.getPreSeedImage(ctx, id, imageType, architecture, tokenSeed.Seeds)
}

func (s tokenService) getPreSeedImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture api.Architecture, seeds TokenImageSeedConfigs) (_ io.ReadCloser, err error) {
	// TODO: Allow filters?
	updates, err := s.updateSvc.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get updates: %w", err)
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("Failed to get updates: No updates found")
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

	rc, err := s.flasher.GenerateSeededImage(ctx, id, seeds, file)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate seeded image: %w", err)
	}

	return rc, nil
}
