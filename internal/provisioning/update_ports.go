package provisioning

import (
	"archive/tar"
	"context"
	"io"

	"github.com/google/uuid"
)

type UpdateService interface {
	SetServerService(serverSvc ServerService)

	GetAll(ctx context.Context) (Updates, error)
	GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, id uuid.UUID) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error)

	CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (uuid.UUID, error)
	CleanupAll(ctx context.Context) error
	Prune(ctx context.Context) error
	Refresh(ctx context.Context) error

	// Exposedchannels
	CreateExposedchannel(ctx context.Context, newExposedchannel Exposedchannel) (Exposedchannel, error)
	GetAllExposedchannels(ctx context.Context) (Exposedchannels, error)
	GetAllExposedchannelNames(ctx context.Context) ([]string, error)
	GetExposedchannelByName(ctx context.Context, name string) (*Exposedchannel, error)
	UpdateExposedchannel(ctx context.Context, newExposedchannel Exposedchannel) error
	RenameExposedchannel(ctx context.Context, oldName string, newName string) error
	DeleteExposedchannelByName(ctx context.Context, name string) error
}

type UpdateRepo interface {
	Upsert(ctx context.Context, update Update) error
	GetAll(ctx context.Context) (Updates, error)
	GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	GetUpdatesByAssignedExposedchannelName(ctx context.Context, name string) (Updates, error)

	CreateExposedchannel(ctx context.Context, newExposedchannel Exposedchannel) (int64, error)
	GetAllExposedchannels(ctx context.Context) (Exposedchannels, error)
	GetAllExposedchannelNames(ctx context.Context) ([]string, error)
	GetExposedchannelByName(ctx context.Context, name string) (*Exposedchannel, error)
	UpdateExposedchannel(ctx context.Context, newExposedchannel Exposedchannel) error
	RenameExposedchannel(ctx context.Context, oldName string, newName string) error
	DeleteExposedchannelByName(ctx context.Context, name string) error
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
