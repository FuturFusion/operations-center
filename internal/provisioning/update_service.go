package provisioning

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/transaction"
)

type updateService struct {
	repo        UpdateRepo
	source      UpdateSourcePort
	latestLimit int
}

var _ UpdateService = &updateService{}

func NewUpdateService(repo UpdateRepo, source UpdateSourcePort, latestLimit int) updateService {
	return updateService{
		repo:        repo,
		source:      source,
		latestLimit: latestLimit,
	}
}

func (s updateService) GetAll(ctx context.Context) (Updates, error) {
	return s.repo.GetAll(ctx)
}

func (s updateService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s updateService) GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error) {
	return s.repo.GetByUUID(ctx, id)
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

	return s.source.GetUpdateFileByFilename(ctx, *update, filename)
}

func (s updateService) Refresh(ctx context.Context) error {
	updates, err := s.source.GetLatest(ctx, s.latestLimit)
	if err != nil {
		return fmt.Errorf("Failed to fetch latest updates from source: %w", err)
	}

	for _, update := range updates {
		updateFiles, err := s.source.GetUpdateAllFiles(ctx, update)
		if err != nil {
			return fmt.Errorf(`Failed to get files for update "%s:%s": %w`, update.Channel, update.Version, err)
		}

		update.Files = updateFiles

		for _, updateFile := range updateFiles {
			if ctx.Err() != nil {
				return fmt.Errorf("stop refresh, context cancelled: %w", context.Cause(ctx))
			}

			err := func() (err error) {
				stream, _, err := s.source.GetUpdateFileByFilename(ctx, update, updateFile.Filename)
				if err != nil {
					return fmt.Errorf(`Failed to fetch file %q for update "%s:%s": %w`, updateFile.Filename, update.Channel, update.Version, err)
				}

				defer func() {
					closeErr := stream.Close()
					if closeErr != nil {
						err = errors.Join(err, fmt.Errorf(`Failed to close stream for file %q of update "%s:%s": %w`, updateFile.Filename, update.Channel, update.Version, closeErr))
					}
				}()

				// We don't care about the actual file content at this stage. We just
				// make sure, we are able to read the file (which causes the caching
				// middleware to download the file if not yet present in the cache).
				_, err = io.ReadAll(stream)
				if err != nil {
					return fmt.Errorf(`Failed to read stream for file %q of update "%s:%s": %w`, updateFile.Filename, update.Channel, update.Version, err)
				}

				return nil
			}()
			if err != nil {
				return err
			}
		}

		err = s.repo.Upsert(ctx, update)
		if err != nil {
			return fmt.Errorf("Failed to persist the update in the repository: %w", err)
		}
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		allUpdates, err := s.repo.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get all updates from repository: %w", err)
		}

		sort.Slice(allUpdates, func(i, j int) bool {
			return allUpdates[i].PublishedAt.After(allUpdates[j].PublishedAt)
		})

		if len(allUpdates) > s.latestLimit {
			for _, update := range allUpdates[s.latestLimit:] {
				err = s.source.ForgetUpdate(ctx, update)
				if err != nil {
					return fmt.Errorf("Failed to forget update %s: %w", update.UUID, err)
				}

				err = s.repo.DeleteByUUID(ctx, update.UUID)
				if err != nil {
					return fmt.Errorf("Failed to remove update %s from repository: %w", update.UUID, err)
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
