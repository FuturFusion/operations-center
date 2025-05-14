package provisioning

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type UpdateService interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, id uuid.UUID) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error)

	Refresh(ctx context.Context) error
}

type UpdateRepo interface {
	Upsert(ctx context.Context, update Update) error
	GetAll(ctx context.Context) (Updates, error)
	GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
}

// A UpdateSourcePort is a source for updates (e.g. IncusOS or HypervisorOS).
type UpdateSourcePort interface {
	GetLatest(ctx context.Context, limit int) (Updates, error)
	GetUpdateAllFiles(ctx context.Context, update Update) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, update Update, filename string) (io.ReadCloser, int, error)
	ForgetUpdate(ctx context.Context, update Update) error
}
