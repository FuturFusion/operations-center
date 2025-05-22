package provisioning

import (
	"archive/tar"
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

	CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (uuid.UUID, error)
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
}

type UpdateSourceWithForgetPort interface {
	UpdateSourcePort
	ForgetUpdate(ctx context.Context, update Update) error
}

type UpdateSourceWithForgetAndAddPort interface {
	UpdateSourceWithForgetPort
	Add(ctx context.Context, tarReader *tar.Reader) (*Update, error)
}
