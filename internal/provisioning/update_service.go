package provisioning

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

const defaultLatestLimit = 3

type updateService struct {
	repo        UpdateRepo
	filesRepo   UpdateFilesRepo
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

func NewUpdateService(repo UpdateRepo, filesRepo UpdateFilesRepo, opts ...UpdateServiceOption) updateService {
	service := updateService{
		repo:        repo,
		filesRepo:   filesRepo,
		source:      make(map[string]UpdateSourcePort),
		latestLimit: defaultLatestLimit,
	}

	for _, opt := range opts {
		opt(&service)
	}

	return service
}

func (s updateService) CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (uuid.UUID, error) {
	update, err := s.filesRepo.CreateFromArchive(ctx, tarReader)
	if err != nil {
		return uuid.UUID{}, err
	}

	update.Status = api.UpdateStatusReady

	err = update.Validate()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("Validate update: %w", err)
	}

	err = s.repo.Upsert(ctx, *update)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("Failed to persist the update from archive in the repository: %w", err)
	}

	return update.UUID, nil
}

func (s updateService) GetAll(ctx context.Context) (Updates, error) {
	updates, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	sort.Sort(updates)

	return updates, nil
}

func (s updateService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s updateService) GetAllWithFilter(ctx context.Context, filter UpdateFilter) (Updates, error) {
	var err error
	var updates Updates

	if filter.UUID == nil && filter.Channel == nil && filter.Origin == nil && filter.Status == nil {
		updates, err = s.repo.GetAll(ctx)
	} else {
		updates, err = s.repo.GetAllWithFilter(ctx, filter)
	}

	if err != nil {
		return nil, err
	}

	sort.Sort(updates)

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

// GetUpdateFileByFilename returns a file of an update from the files repository.
//
// GetUpdateFileByFilename returns an io.ReadCloser that reads the contents of
// the specified release asset.
// It is the caller's responsibility to close the ReadCloser.
func (s updateService) GetUpdateFileByFilename(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
	update, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, 0, err
	}

	found := false
	for _, file := range update.Files {
		if filename == file.Filename {
			found = true
			break
		}
	}

	if !found {
		return nil, 0, fmt.Errorf("Requested file %q is not part of update %q", filename, id.String())
	}

	return s.filesRepo.Get(ctx, *update, filename)
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
		var found bool
		err = transaction.Do(ctx, func(ctx context.Context) error {
			_, err := s.repo.GetByUUID(ctx, update.UUID)
			if err == nil {
				// Update is already in the DB.
				found = true
				return nil
			}

			if !errors.Is(err, domain.ErrNotFound) {
				return err
			}

			// Overwrite origin with our value to ensure cleanup to work.
			update.Origin = origin
			update.Status = api.UpdateStatusPending

			err = update.Validate()
			if err != nil {
				return fmt.Errorf("Validate update: %w", err)
			}

			return s.repo.Upsert(ctx, update)
		})
		if err != nil {
			return fmt.Errorf("Failed to create update in pending state: %w", err)
		}

		if found {
			continue
		}

		var requiredSpaceTotal int
		for _, file := range update.Files {
			requiredSpaceTotal += file.Size
		}

		ui, err := s.filesRepo.UsageInformation(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get usage information: %w", err)
		}

		if (float64(ui.AvailableSpaceBytes)-float64(requiredSpaceTotal))/float64(ui.TotalSpaceBytes) < 0.1 {
			return fmt.Errorf("Not enough space available in files repository, require: %d, available: %d, required headroom after download: 10%%", requiredSpaceTotal, ui.AvailableSpaceBytes)
		}

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
				var stream io.ReadCloser
				stream, _, err = src.GetUpdateFileByFilenameUnverified(ctx, update, updateFile.Filename)
				if err != nil {
					return fmt.Errorf(`Failed to fetch update file "%s:%s/%s@%s": %w`, origin, update.Channel, updateFile.Filename, update.Version, err)
				}

				teeStream := stream
				var h hash.Hash

				if updateFile.Sha256 != "" {
					h = sha256.New()
					teeStream = newTeeReadCloser(stream, h)
				}

				commit, cancel, err := s.filesRepo.Put(ctx, update, updateFile.Filename, teeStream)
				if err != nil {
					return fmt.Errorf(`Failed to read stream for update file "%s:%s/%s@%s": %w`, origin, update.Channel, updateFile.Filename, update.Version, err)
				}

				defer func() {
					cancelErr := cancel()
					if cancelErr != nil {
						err = errors.Join(err, cancelErr)
					}
				}()

				if updateFile.Sha256 != "" {
					checksum := hex.EncodeToString(h.Sum(nil))
					if updateFile.Sha256 != checksum {
						return fmt.Errorf("Invalid update, file sha256 mismatch for file %q, manifest: %s, actual: %s", updateFile.Filename, updateFile.Sha256, checksum)
					}
				}

				return commit()
			}()
			if err != nil {
				return err
			}
		}

		update.Status = api.UpdateStatusReady

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
				err = s.filesRepo.Delete(ctx, update)
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
