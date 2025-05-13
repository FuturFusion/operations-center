package provisioning

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type UpdateService interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, id uuid.UUID) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error)
}

type UpdateRepo interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)
}

	// Files
	GetUpdateAllFiles(ctx context.Context, updateID string) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error)
}
