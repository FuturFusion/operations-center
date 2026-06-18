package image

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

type incusSourceService struct {
	repo             IncusImageSourceRepo
	incusImageSource IncusImageSourcePort
}

var _ IncusImageSourceService = &incusSourceService{}

func NewIncusSource(repo IncusImageSourceRepo, incusImageSource IncusImageSourcePort) *incusSourceService {
	sourceSvc := &incusSourceService{
		repo:             repo,
		incusImageSource: incusImageSource,
	}

	return sourceSvc
}

func (s *incusSourceService) Create(ctx context.Context, newSource IncusImageSource) (IncusImageSource, error) {
	err := newSource.Validate(ctx, s.incusImageSource)
	if err != nil {
		return IncusImageSource{}, fmt.Errorf("Validation failed for new image source: %w", err)
	}

	newSource.ID, err = s.repo.Create(ctx, newSource)
	if err != nil {
		return IncusImageSource{}, fmt.Errorf("Failed to create image source: %w", err)
	}

	return newSource, nil
}

func (s *incusSourceService) GetAll(ctx context.Context) (IncusImageSources, error) {
	sources, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get image sources: %w", err)
	}

	return sources, nil
}

func (s *incusSourceService) GetAllNames(ctx context.Context) ([]string, error) {
	names, err := s.repo.GetAllNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to get image source names: %w", err)
	}

	return names, nil
}

func (s *incusSourceService) GetByName(ctx context.Context, name string) (*IncusImageSource, error) {
	if name == "" {
		return nil, fmt.Errorf("Image source name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	source, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get image source %q: %w", name, err)
	}

	return source, nil
}

func (s *incusSourceService) Update(ctx context.Context, newSource IncusImageSource) error {
	err := newSource.Validate(ctx, s.incusImageSource)
	if err != nil {
		return fmt.Errorf("Validation failed for image source: %w", err)
	}

	return s.repo.Update(ctx, newSource)
}

func (s *incusSourceService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Image source name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := transaction.Do(ctx, func(ctx context.Context) error {
		err := s.incusImageSource.DeleteBySource(ctx, name)
		if err != nil {
			return err
		}

		err = s.repo.DeleteByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete image source %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *incusSourceService) RefreshByName(ctx context.Context, name string) error {
	source, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Sync sources failed to sources: %w", err)
	}

	err = s.incusImageSource.RefreshFromSource(ctx, *source)
	if err != nil {
		return fmt.Errorf("Sync source %q failed: %w", source.Name, err)
	}

	return nil
}

func (s *incusSourceService) RefreshAll(ctx context.Context) error {
	sources, err := s.repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Sync sources failed to sources: %w", err)
	}

	for _, source := range sources {
		err = s.incusImageSource.RefreshFromSource(ctx, source)
		if err != nil {
			return fmt.Errorf("Sync source %q failed: %w", source.Name, err)
		}
	}

	return nil
}
