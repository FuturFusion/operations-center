package provisioning

import (
	"context"
	"io"
)

type UpdateService interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllIDs(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id string) (Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, updateID string) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error)
}

type UpdateRepo interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllIDs(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id string) (Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, updateID string) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error)
}
