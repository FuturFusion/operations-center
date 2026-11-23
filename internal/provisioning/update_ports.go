package provisioning

import (
	"archive/tar"
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/shared/api"
)

type UpdateService interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)
	GetUpdatesByAssignedChannelName(ctx context.Context, channelName string) (Updates, error)
	Update(ctx context.Context, update Update) error
	GetChangelog(ctx context.Context, currentID uuid.UUID, priorID uuid.UUID, architecture images.UpdateFileArchitecture) (api.UpdateChangelog, error)
	GetChangelogByChannel(ctx context.Context, UUID uuid.UUID, channelName string, upstream bool, architecture images.UpdateFileArchitecture) (api.UpdateChangelog, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, id uuid.UUID) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error)

	CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (uuid.UUID, error)
	CleanupAll(ctx context.Context) error
	Prune(ctx context.Context) error
	Refresh(ctx context.Context) error

	RefreshLocalFiles(ctx context.Context, id uuid.UUID) error
	CheckLocalFiles(ctx context.Context, updates Updates) ([]uuid.UUID, error)

	SetServerService(serverSvc ServerService)
}

type UpdateRepo interface {
	Upsert(ctx context.Context, update Update) error
	GetAll(ctx context.Context) (Updates, error)
	GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error)
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	GetUpdatesByAssignedChannelName(ctx context.Context, name string, filter ...UpdateFilter) (Updates, error)
	AssignChannels(ctx context.Context, id uuid.UUID, channelNames []string) error
}

type (
	CommitFunc func() error
	CancelFunc func() error
)

type UpdateFilesRepo interface {
	Exists(ctx context.Context, update Update, filename string) (bool, error)
	Get(ctx context.Context, update Update, filename string) (_ io.ReadCloser, size int, _ error)
	Put(ctx context.Context, update Update, filename string, content io.ReadCloser) (CommitFunc, CancelFunc, error)
	Delete(ctx context.Context, update Update) error
	PruneFiles(ctx context.Context, update Update) (_ error)
	UsageInformation(ctx context.Context) (file.UsageInformation, error)
	CleanupAll(ctx context.Context) error
	CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (*Update, error)
}

// UpdateFileTargetPort lets other cluster members manage the update files they store.
type UpdateFileTargetPort interface {
	// LocalMemberID returns the ID of this member.
	LocalMemberID(ctx context.Context) (int64, error)

	// GetFile returns a file of update from the member storing it.
	GetFile(ctx context.Context, update Update, filename string) (_ io.ReadCloser, size int, _ error)

	// RefreshFiles lets the member storing update download any missing files and delete the files it no longer needs.
	RefreshFiles(ctx context.Context, update Update) error

	// DeleteFiles lets the member storing update delete its files.
	DeleteFiles(ctx context.Context, update Update) error

	// CheckFiles returns the updates whose files on the member memberID are incomplete.
	CheckFiles(ctx context.Context, memberID int64, updates Updates) ([]uuid.UUID, error)

	// DeleteAllFiles lets all other members delete the files of all updates.
	DeleteAllFiles(ctx context.Context) error
}

// A UpdateSourcePort is a source for updates (e.g. IncusOS or HypervisorOS).
type UpdateSourcePort interface {
	GetLatest(ctx context.Context, limit int) (Updates, error)
	GetUpdateFileByFilenameUnverified(ctx context.Context, update Update, filename string) (io.ReadCloser, int, error)
}
