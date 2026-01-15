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
	GetUpdatesByAssignedChannelName(ctx context.Context, channelName string) (Updates, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, id uuid.UUID) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error)

	CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (uuid.UUID, error)
	CleanupAll(ctx context.Context) error
	Prune(ctx context.Context) error
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
	GetUpdatesByAssignedChannelName(ctx context.Context, name string) (Updates, error)
}

type (
	CommitFunc func() error
	CancelFunc func() error
)

type UpdateFilesRepo interface {
	Get(ctx context.Context, update Update, filename string) (_ io.ReadCloser, size int, _ error)
	Put(ctx context.Context, update Update, filename string, content io.ReadCloser) (CommitFunc, CancelFunc, error)
	Delete(ctx context.Context, update Update) error
	UsageInformation(ctx context.Context) (UsageInformation, error)
	CleanupAll(ctx context.Context) error
	CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (*Update, error)
}

// A UpdateSourcePort is a source for updates (e.g. IncusOS or HypervisorOS).
type UpdateSourcePort interface {
	GetLatest(ctx context.Context, limit int) (Updates, error)
	GetUpdateFileByFilenameUnverified(ctx context.Context, update Update, filename string) (io.ReadCloser, int, error)
}
