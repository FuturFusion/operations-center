package provisioning

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

const defaultLatestLimit = 3

type updateService struct {
	repo        UpdateRepo
	source      map[string]UpdateSourcePort
	latestLimit int
}

var _ UpdateService = &updateService{}

type UpdateServiceOption func(service *updateService)

func UpdateServiceWithSource(origin string, source UpdateSourcePort) UpdateServiceOption {
	return func(service *updateService) {
		service.source[origin] = source
	}
}

func UpdateServiceWithLatestLimit(limit int) UpdateServiceOption {
	return func(service *updateService) {
		service.latestLimit = limit
	}
}

func NewUpdateService(repo UpdateRepo, opts ...UpdateServiceOption) updateService {
	service := updateService{
		repo:        repo,
		source:      make(map[string]UpdateSourcePort),
		latestLimit: defaultLatestLimit,
	}

	for _, opt := range opts {
		opt(&service)
	}

	return service
}

func (s updateService) CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (uuid.UUID, error) {
	var src UpdateSourceWithAddPort

	var found bool
	var origin string
	for o, s := range s.source {
		src, found = s.(UpdateSourceWithAddPort)
		if found {
			origin = o
			break
		}
	}

	if !found {
		return uuid.UUID{}, fmt.Errorf("Operation not supported, no update source allows manual update")
	}

	update, err := src.Add(ctx, tarReader)
	if err != nil {
		return uuid.UUID{}, err
	}

	err = s.refreshOrigin(ctx, origin, src)
	if err != nil {
		return update.UUID, fmt.Errorf("Failed to refresh manual updates: %w", err)
	}

	return update.UUID, nil
}

func (s updateService) GetAll(ctx context.Context) (Updates, error) {
	return s.repo.GetAll(ctx)
}

func (s updateService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s updateService) GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error) {
	var err error
	var updates Updates

	if filter.UUID == nil && filter.Channel == nil {
		updates, err = s.repo.GetAll(ctx)
	} else {
		updates, err = s.repo.GetAllWithFilter(ctx, filter)
	}

	if err != nil {
		return nil, err
	}

	return updates, nil
}

func (s updateService) GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s updateService) GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error) {
	var err error
	var updateIDs []uuid.UUID

	if filter.UUID == nil && filter.Channel == nil {
		updateIDs, err = s.repo.GetAllUUIDs(ctx)
	} else {
		updateIDs, err = s.repo.GetAllUUIDsWithFilter(ctx, filter)
	}

	if err != nil {
		return nil, err
	}

	return updateIDs, nil
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

	src, ok := s.source[update.Origin]
	if !ok {
		return nil, 0, fmt.Errorf("Unsupported origin %q", update.Origin)
	}

	return src.GetUpdateFileByFilename(ctx, *update, filename)
}

func (s updateService) Refresh(ctx context.Context) error {
	for origin, source := range s.source {
		err := s.refreshOrigin(ctx, origin, source)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s updateService) refreshOrigin(ctx context.Context, origin string, src UpdateSourcePort) error {
	updates, err := src.GetLatest(ctx, s.latestLimit)
	if err != nil {
		return fmt.Errorf("Failed to fetch latest updates from source %q: %w", origin, err)
	}

	for _, update := range updates {
		updateFiles, err := src.GetUpdateAllFiles(ctx, update)
		if err != nil {
			return fmt.Errorf(`Failed to get files for update "%s:%s@%s": %w`, origin, update.Channel, update.Version, err)
		}

		update.Files = updateFiles

		for _, updateFile := range updateFiles {
			if ctx.Err() != nil {
				return fmt.Errorf("Stop refresh, context cancelled: %w", context.Cause(ctx))
			}

			err := func() (err error) {
				stream, _, err := src.GetUpdateFileByFilename(ctx, update, updateFile.Filename)
				if err != nil {
					return fmt.Errorf(`Failed to fetch update file "%s:%s/%s@%s": %w`, origin, update.Channel, updateFile.Filename, update.Version, err)
				}

				defer func() {
					closeErr := stream.Close()
					if closeErr != nil {
						err = errors.Join(err, fmt.Errorf(`Failed to close stream for update file "%s:%s/%s@%s": %w`, origin, update.Channel, updateFile.Filename, update.Version, closeErr))
					}
				}()

				// We don't care about the actual file content at this stage. We just
				// make sure, we are able to read the file (which causes the caching
				// middleware to download the file if not yet present in the cache).
				_, err = io.ReadAll(stream)
				if err != nil {
					return fmt.Errorf(`Failed to read stream for update file "%s:%s/%s@%s": %w`, origin, update.Channel, updateFile.Filename, update.Version, err)
				}

				return nil
			}()
			if err != nil {
				return err
			}
		}

		err = s.repo.Upsert(ctx, update)
		if err != nil {
			return fmt.Errorf("Failed to persist the update in the repository for source %q: %w", origin, err)
		}
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		allUpdates, err := s.repo.GetAllWithFilter(ctx, UpdateFilter{
			Origin: ptr.To(origin),
		})
		if err != nil {
			return fmt.Errorf("Failed to get all updates from repository for source %q: %w", origin, err)
		}

		sort.Slice(allUpdates, func(i, j int) bool {
			return allUpdates[i].PublishedAt.After(allUpdates[j].PublishedAt)
		})

		if len(allUpdates) > s.latestLimit {
			for _, update := range allUpdates[s.latestLimit:] {
				err = src.ForgetUpdate(ctx, update)
				if err != nil {
					return fmt.Errorf("Failed to forget update %s from source %q: %w", update.UUID, origin, err)
				}

				err = s.repo.DeleteByUUID(ctx, update.UUID)
				if err != nil {
					return fmt.Errorf("Failed to remove update %s from source %q from repository: %w", update.UUID, origin, err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("Unable to refresh updates from source: %w", err)
	}

	return nil
}
