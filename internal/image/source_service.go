package image

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type sourceService struct {
	repo          SourceRepo
	imageSourcers map[api.ImageSourceType]ImageSourcerPort
}

var _ SourceService = &sourceService{}

func NewSource(repo SourceRepo, imageSourcers map[api.ImageSourceType]ImageSourcerPort) *sourceService {
	sourceSvc := &sourceService{
		repo:          repo,
		imageSourcers: imageSourcers,
	}

	return sourceSvc
}

func (s *sourceService) Create(ctx context.Context, newSource ImageSource) (ImageSource, error) {
	err := newSource.Validate(ctx, s.imageSourcers)
	if err != nil {
		return ImageSource{}, fmt.Errorf("Validation failed for new image source: %w", err)
	}

	newSource.ID, err = s.repo.Create(ctx, newSource)
	if err != nil {
		return ImageSource{}, fmt.Errorf("Failed to create image source: %w", err)
	}

	return newSource, nil
}

func (s *sourceService) GetAll(ctx context.Context) (Sources, error) {
	sources, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get image sources: %w", err)
	}

	return sources, nil
}

func (s *sourceService) GetAllNames(ctx context.Context) ([]string, error) {
	names, err := s.repo.GetAllNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get image source names: %w", err)
	}

	return names, nil
}

func (s *sourceService) GetByName(ctx context.Context, name string) (*ImageSource, error) {
	if name == "" {
		return nil, fmt.Errorf("Image source name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	source, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get image source %q: %w", name, err)
	}

	return source, nil
}

func (s *sourceService) Update(ctx context.Context, newSource ImageSource) error {
	err := newSource.Validate(ctx, s.imageSourcers)
	if err != nil {
		return fmt.Errorf("Validation failed for image source: %w", err)
	}

	return s.repo.Update(ctx, newSource)
}

func (s *sourceService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Image source name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// FIXME: remove all images from the file repository through the respective service (e.g. incus image)

	err := s.repo.DeleteByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to delete image source %q: %w", name, err)
	}

	return nil
}

func (s *sourceService) SyncSources(ctx context.Context) error {
	sources, err := s.repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Sync sources failed to sources: %w", err)
	}

	for _, source := range sources {
		imageSourcer, ok := s.imageSourcers[source.Type]
		if !ok {
			return fmt.Errorf("No source implementation available for source type %q", source.Type)
		}

		err = imageSourcer.RefreshFromSource(ctx, source)
		if err != nil {
			return fmt.Errorf("Sync source %q failed: %w", source.Name, err)
		}
	}

	return nil
}
