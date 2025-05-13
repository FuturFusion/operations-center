package provisioning

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type updateService struct {
	repo UpdateRepo
}

var _ UpdateService = &updateService{}

func NewUpdateService(repo UpdateRepo) updateService {
	return updateService{
		repo: repo,
	}
}

func (s updateService) GetAll(ctx context.Context) (Updates, error) {
	return s.repo.GetAll(ctx)
}

func (s updateService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s updateService) GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s updateService) GetUpdateAllFiles(ctx context.Context, id uuid.UUID) (UpdateFiles, error) {
	update, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, err
	}

	return update.Files, nil
}

// GetUpdateFileByFilename downloads a file of an update.
//
// GetUpdateFileByFilename returns an io.ReadCloser that reads the contents of the specified release asset.
// It is the caller's responsibility to close the ReadCloser.
func (s updateService) GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
	update, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, 0, err
	}

	return s.source.GetUpdateFileByFilename(ctx, *update, filename)
}
