package provisioning

import (
	"context"
	"io"
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

func (s updateService) GetAllIDs(ctx context.Context) ([]string, error) {
	return s.repo.GetAllIDs(ctx)
}

func (s updateService) GetByID(ctx context.Context, id string) (Update, error) {
	return s.repo.GetByID(ctx, id)
}

func (s updateService) GetUpdateAllFiles(ctx context.Context, updateID string) (UpdateFiles, error) {
	return s.repo.GetUpdateAllFiles(ctx, updateID)
}

// GetUpdateFileByFilename downloads a file of an update.
//
// GetUpdateFileByFilename returns an io.ReadCloser that reads the contents of the specified release asset.
// It is the caller's responsibility to close the ReadCloser.
func (s updateService) GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error) {
	return s.repo.GetUpdateFileByFilename(ctx, updateID, filename)
}
