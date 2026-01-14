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
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/google/uuid"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	defaultFetchLimit       = 10
	defaultLatestLimit      = 3
	defaultPendingGraceTime = 24 * time.Hour
)

type updateService struct {
	configUpdateMu *sync.Mutex

	repo                       UpdateRepo
	filesRepo                  UpdateFilesRepo
	source                     UpdateSourcePort
	serverSvc                  ServerService
	latestLimit                int
	updateFilterExpression     string
	updateFileFilterExpression string
	pendingGracePeriod         time.Duration
}

var _ UpdateService = &updateService{}

type UpdateServiceOption func(service *updateService)

func UpdateServiceWithLatestLimit(limit int) UpdateServiceOption {
	return func(service *updateService) {
		service.latestLimit = limit
	}
}

func UpdateServiceWithPendingGracePeriod(pendingGracePeriod time.Duration) UpdateServiceOption {
	return func(service *updateService) {
		service.pendingGracePeriod = pendingGracePeriod
	}
}

func UpdateServiceWithFilterExpression(filterExpression string) UpdateServiceOption {
	return func(service *updateService) {
		service.updateFilterExpression = filterExpression
	}
}

func UpdateServiceWithFileFilterExpression(filesFilterExpression string) UpdateServiceOption {
	return func(service *updateService) {
		service.updateFileFilterExpression = filesFilterExpression
	}
}

func NewUpdateService(repo UpdateRepo, filesRepo UpdateFilesRepo, source UpdateSourcePort, opts ...UpdateServiceOption) *updateService {
	service := &updateService{
		configUpdateMu: &sync.Mutex{},

		repo:               repo,
		filesRepo:          filesRepo,
		source:             source,
		latestLimit:        defaultLatestLimit,
		pendingGracePeriod: defaultPendingGraceTime,
	}

	for _, opt := range opts {
		opt(service)
	}

	// Register for the UpdatesValidateSignal to validate the updates filter
	// expression and the updates file filter expression.
	// The way through signals is chosen here to prevent a dependency cycle
	// between the config and the provisioning package.
	config.UpdatesValidateSignal.AddListenerWithErr(service.validateUpdatesConfig)

	return service
}

func (s *updateService) SetServerService(serverSvc ServerService) {
	s.serverSvc = serverSvc
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

func (s updateService) CleanupAll(ctx context.Context) error {
	// Since we are going to delete all the updates anyway and because this
	// method is intended to be an escape hatch, which should also work, if
	// the disk is completely full and therefore writes to the DB would likely fail,
	// the updates are removed first and only after the DB is updated.
	err := s.filesRepo.CleanupAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to cleanup: %w", err)
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		updates, err := s.repo.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get all updates during cleanup: %w", err)
		}

		for _, update := range updates {
			err = s.repo.DeleteByUUID(ctx, update.UUID)
			if err != nil {
				return fmt.Errorf("Failed to delete update %v: %w", update.UUID, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// Prune ensures, that incomplete updates are removed and with this sets a clean
// stage for a subsequent refresh. Prune is normally only called on startup
// of the service.
// Prune removes the following updates:
//
//   - Updates, that are in pending state (most likely caused by shutdown of
//     the service or network interrupts while a refresh operation has been in
//     process.
//   - Updates in ready state, where files are missing (most likely caused
//     by a restore of the application's backuped state by IncusOS.
func (s updateService) Prune(ctx context.Context) error {
	var fileRepoErrs []error

	err := transaction.Do(ctx, func(ctx context.Context) error {
		updates, err := s.repo.GetAllWithFilter(ctx, UpdateFilter{
			Status: ptr.To(api.UpdateStatusPending),
		})
		if err != nil {
			return fmt.Errorf("Failed to get all pending updates during prune: %w", err)
		}

		for _, update := range updates {
			remove := false

			switch update.Status {
			case api.UpdateStatusPending:
				remove = true

			case api.UpdateStatusReady:
				for _, file := range update.Files {
					rc, size, err := s.filesRepo.Get(ctx, update, file.Filename)
					if rc != nil {
						_ = rc.Close()
					}

					if err != nil || file.Size != size {
						// TODO: currently, we only check if the file exist and the file size
						// matches. We could be extra careful and also check if the hash
						// is correct, but this would be significantly slower and would
						// cause startup of the daemon to be significantly slower.
						remove = true
						break
					}
				}
			}

			if !remove {
				continue
			}

			err = s.filesRepo.Delete(ctx, update)
			if err != nil {
				fileRepoErrs = append(fileRepoErrs, fmt.Errorf("Failed to remove files of update %q: %w", update.UUID.String(), err))
			}

			err = s.repo.DeleteByUUID(ctx, update.UUID)
			if err != nil {
				return fmt.Errorf("Failed to delete update %v: %w", update.UUID, err)
			}
		}

		return nil
	})
	err = errors.Join(append([]error{err}, fileRepoErrs...)...)
	if err != nil {
		return fmt.Errorf("Failed to prune pending updates: %w", err)
	}

	return nil
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

	if filter.UUID == nil && filter.Origin == nil && filter.Status == nil {
		updates, err = s.repo.GetAll(ctx)
	} else {
		updates, err = s.repo.GetAllWithFilter(ctx, filter)
	}

	if err != nil {
		return nil, err
	}

	sort.Sort(updates)

	if filter.Channel == nil {
		return updates, nil
	}

	n := 0
	for i := range updates {
		if !slices.Contains(updates[i].Channels, *filter.Channel) {
			continue
		}

		updates[n] = updates[i]
		n++
	}

	return updates[:n], nil
}

func (s updateService) GetByUUID(ctx context.Context, id uuid.UUID) (*Update, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s updateService) GetAllUUIDsWithFilter(ctx context.Context, filter UpdateFilter) ([]uuid.UUID, error) {
	if filter.Channel == nil {
		updateIDs, err := s.repo.GetAllUUIDs(ctx)
		if err != nil {
			return nil, err
		}

		return updateIDs, nil
	}

	updates, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	updateIDs := make([]uuid.UUID, 0, len(updates))
	for _, update := range updates {
		if !slices.Contains(update.Channels, *filter.Channel) {
			continue
		}

		updateIDs = append(updateIDs, update.UUID)
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

// Refresh refreshes the updates from an origin.
//
// This operations is performed in the following steps:
//
//   - Get latest updates (up to the defined limit) from the origin.
//   - Get all existing updates for the respective origin from the DB.
//   - Merge the two sets such that updates already present in the DB take precedence over same updates from origin.
//   - Determine the resulting state using the following logic:
//     Sort the merged list of updates by published at date in descending order.
//     Pending updates are not considered. If pending updates are in pending state for more than `pendingGraceTime`, these updates are removed.
//     At least the most recent update currently available in the DB is kept.
//     Select the n most recent updates from the merged list, where n is defined by the parameter `latestLimit`.
//     The remainder of the updates are omitted (ignored, if not yet downloaded, deleted if already present in the DB).
//   - Remove the updates, which are marked for removal.
//   - Download the updates, that are part of the resulting state and not yet present on the system.
func (s updateService) Refresh(ctx context.Context) error {
	originUpdates, err := s.source.GetLatest(ctx, defaultFetchLimit)
	if err != nil {
		return fmt.Errorf("Failed to fetch latest updates: %w", err)
	}

	// Filter updates from orign by filter expression.
	originUpdates, err = s.filterUpdatesByFilterExpression(originUpdates)
	if err != nil {
		return err
	}

	// Filter update files by architecture.
	originUpdates, err = s.filterUpdateFileByFilterExpression(originUpdates)
	if err != nil {
		return err
	}

	toDownloadUpdates := make([]Update, 0, len(originUpdates))
	err = transaction.Do(ctx, func(ctx context.Context) error {
		dbUpdates, err := s.repo.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get all updates from repository: %w", err)
		}

		var toDeleteUpdates []Update
		toDeleteUpdates, toDownloadUpdates = s.determineToDeleteAndToDownloadUpdates(dbUpdates, originUpdates)

		// Remove updates marked for removal.
		for _, update := range toDeleteUpdates {
			err = s.filesRepo.Delete(ctx, update)
			if err != nil {
				return fmt.Errorf("Failed to forget update %s: %w", update.UUID, err)
			}

			err = s.repo.DeleteByUUID(ctx, update.UUID)
			if err != nil {
				return fmt.Errorf("Failed to remove update %s from repository: %w", update.UUID, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("Unable to refresh updates from source: %w", err)
	}

	if len(toDownloadUpdates) > 0 {
		// Make sure, we do have enough space left in the files repository before moving the state to pending.
		err = s.isSpaceAvailable(ctx, toDownloadUpdates)
		if err != nil {
			return err
		}

		// Move updates marked for download in pending state.
		for i, update := range toDownloadUpdates {
			// Overwrite origin with our value to ensure cleanup to work.
			update.Status = api.UpdateStatusPending

			err = update.Validate()
			if err != nil {
				return fmt.Errorf("Validate update: %w", err)
			}

			toDownloadUpdates[i] = update

			err = s.repo.Upsert(ctx, update)
			if err != nil {
				return fmt.Errorf("Failed to move update in pending state: %w", err)
			}
		}
	}

	for _, update := range toDownloadUpdates {
		// Make sure, we do have enough space left in the files repository before downloading the files.
		err = s.isSpaceAvailable(ctx, []Update{update})
		if err != nil {
			return err
		}

		for _, updateFile := range update.Files {
			if ctx.Err() != nil {
				return fmt.Errorf("Stop refresh, context cancelled: %w", context.Cause(ctx))
			}

			err := func() (err error) {
				var stream io.ReadCloser
				stream, _, err = s.source.GetUpdateFileByFilenameUnverified(ctx, update, updateFile.Filename)
				if err != nil {
					return fmt.Errorf(`Failed to fetch update file "%s@%s": %w`, updateFile.Filename, update.Version, err)
				}

				teeStream := stream
				var h hash.Hash

				if updateFile.Sha256 != "" {
					h = sha256.New()
					teeStream = newTeeReadCloser(stream, h)
				}

				commit, cancel, err := s.filesRepo.Put(ctx, update, updateFile.Filename, teeStream)
				if err != nil {
					return fmt.Errorf(`Failed to read stream for update file "%s@%s": %w`, updateFile.Filename, update.Version, err)
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
			return fmt.Errorf("Failed to persist the update in the repository: %w", err)
		}
	}

	return nil
}

func (s updateService) validateUpdatesConfig(ctx context.Context, su api.SystemUpdates) error {
	if su.FilterExpression != "" {
		_, err := expr.Compile(su.FilterExpression, expr.Env(ToExprUpdate(Update{})))
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, failed to compile filter expression: %v`, err)
		}
	}

	if su.FileFilterExpression != "" {
		_, err := expr.Compile(su.FileFilterExpression, UpdateFileExprEnvFrom(UpdateFile{}).ExprCompileOptions()...)
		if err != nil {
			return domain.NewValidationErrf(`Invalid config, failed to compile file filter expression: %v`, err)
		}
	}

	return nil
}

func (s updateService) filterUpdatesByFilterExpression(updates Updates) (Updates, error) {
	if s.updateFilterExpression != "" {
		filterExpression, err := expr.Compile(s.updateFilterExpression, expr.Env(ToExprUpdate(Update{})))
		if err != nil {
			return nil, fmt.Errorf("Failed to compile filter expression: %w", err)
		}

		n := 0
		for i := range updates {
			output, err := expr.Run(filterExpression, ToExprUpdate(updates[i]))
			if err != nil {
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Filter expression %q does not evaluate to boolean result: %v", s.updateFilterExpression, output)
			}

			if !result {
				continue
			}

			updates[n] = updates[i]
			n++
		}

		updates = updates[:n]
	}

	return updates, nil
}

type UpdateFileExprEnv struct {
	Filename     string `expr:"file_name"`
	Size         int    `expr:"size"`
	Sha256       string `expr:"sha256"`
	Component    string `expr:"component"`
	Type         string `expr:"type"`
	Architecture string `expr:"architecture"`
}

func (u UpdateFileExprEnv) ExprCompileOptions() []expr.Option {
	return []expr.Option{
		expr.Function("applies_to_architecture", func(params ...any) (any, error) {
			if len(params) < 2 {
				return nil, fmt.Errorf("Invalid number of arguments to 'applies_to_architecture', expected <architecture> <expected_architecture>..., where <expected_architecture> is required at least once, got %d argument", len(params))
			}

			// Validate the arguments.
			arch, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("Invalid first argument type to 'applies_to_architecture', expected string, got: %T", params[0])
			}

			wantArchs := make([]string, 0, len(params)-1)
			for i, param := range params[1:] {
				wantArch, ok := param.(string)
				if !ok {
					return nil, fmt.Errorf("Invalid %d argument type to 'applies_to_architecture', expected string, got: %T", i+2, param)
				}

				wantArchs = append(wantArchs, wantArch)
			}

			// Short cirquit if the provided architecture is empty (architecture agnostic).
			if arch == "" {
				return true, nil
			}

			for _, wantArch := range wantArchs {
				if arch == wantArch {
					return true, nil
				}
			}

			return false, nil
		}),

		// Always compile with an empty struct for consistency.
		expr.Env(UpdateFileExprEnv{}),
	}
}

func UpdateFileExprEnvFrom(u UpdateFile) UpdateFileExprEnv {
	return UpdateFileExprEnv{
		Filename:     u.Filename,
		Size:         u.Size,
		Sha256:       u.Sha256,
		Component:    string(u.Component),
		Type:         string(u.Type),
		Architecture: string(u.Architecture),
	}
}

func (s updateService) filterUpdateFileByFilterExpression(updates Updates) (Updates, error) {
	if len(s.updateFileFilterExpression) > 0 {
		fileFilterExpression, err := expr.Compile(s.updateFileFilterExpression, UpdateFileExprEnvFrom(UpdateFile{}).ExprCompileOptions()...)
		if err != nil {
			return nil, fmt.Errorf("Failed to compile file filter expression: %w", err)
		}

		for i := range updates {
			n := 0
			for j := range updates[i].Files {
				output, err := expr.Run(fileFilterExpression, UpdateFileExprEnvFrom(updates[i].Files[j]))
				if err != nil {
					return nil, err
				}

				result, ok := output.(bool)
				if !ok {
					return nil, fmt.Errorf("File filter expression %q does not evaluate to boolean result: %v", s.updateFilterExpression, output)
				}

				if !result {
					continue
				}

				updates[i].Files[n] = updates[i].Files[j]
				n++
			}

			updates[i].Files = updates[i].Files[:n]
		}
	}

	return updates, nil
}

func (s updateService) determineToDeleteAndToDownloadUpdates(dbUpdates []Update, originUpdates []Update) (toDeleteUpdates []Update, toDownloadUpdates []Update) {
	// Merge dbUpdates and originUpdates to the desired end state.
	mergedUpdates := make([]Update, 0, len(dbUpdates)+len(originUpdates))
	mergedUpdates = append(mergedUpdates, dbUpdates...)
	for _, originUpdate := range originUpdates {
		// Add updates from origin to the merged updates list, if they are not yet present.
		var found bool
		for _, update := range mergedUpdates {
			if originUpdate.UUID == update.UUID {
				found = true
				break
			}
		}

		if !found {
			mergedUpdates = append(mergedUpdates, originUpdate)
		}
	}

	// Make sure, all updates are sorted by published at date.
	sort.Slice(mergedUpdates, func(i, j int) bool {
		return mergedUpdates[i].PublishedAt.After(mergedUpdates[j].PublishedAt)
	})

	// If there are currently no updates in the DB, we don't need to reserve
	// a slot for the most recent update from the DB.
	mostRecentInDBFound := len(dbUpdates) == 0

	toDeleteUpdates = make([]Update, 0, len(dbUpdates))
	toDownloadUpdates = make([]Update, 0, len(originUpdates))
	updateCount := 0
	for _, update := range mergedUpdates {
		// Mark updates in state pending for more than the defined grace time for deletion.
		if update.Status == api.UpdateStatusPending && time.Since(update.LastUpdated) > s.pendingGracePeriod {
			toDeleteUpdates = append(toDeleteUpdates, update)
			continue
		}

		switch update.Status {
		case api.UpdateStatusReady:
			// Update from the DB, already downloaded.
			if updateCount >= s.latestLimit {
				// Already enough updates, mark the remaining ones for removal.
				toDeleteUpdates = append(toDeleteUpdates, update)
				continue
			}

			mostRecentInDBFound = true
			updateCount++

		case api.UpdateStatusUnknown:
			mostRecentInDBHeadroom := 0
			if !mostRecentInDBFound {
				// If we have not yet found the most recent one from the DB, we keep one
				// slot as headroom.
				mostRecentInDBHeadroom = 1
			}

			if updateCount+mostRecentInDBHeadroom >= s.latestLimit {
				continue
			}

			toDownloadUpdates = append(toDownloadUpdates, update)
			updateCount++

		default:
			// Unlikely to happen, this would be an update in state pending, younger than grace time
			// so effectively an update the is fetched right now.
		}
	}

	return toDeleteUpdates, toDownloadUpdates
}

func (s updateService) isSpaceAvailable(ctx context.Context, downloadUpdates []Update) error {
	var requiredSpaceTotal int
	for _, update := range downloadUpdates {
		for _, file := range update.Files {
			requiredSpaceTotal += file.Size
		}
	}

	ui, err := s.filesRepo.UsageInformation(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get usage information: %w", err)
	}

	if ui.TotalSpaceBytes < 1 {
		return fmt.Errorf("Files repository reported an invalid total space: %d", ui.TotalSpaceBytes)
	}

	if (float64(ui.AvailableSpaceBytes)-float64(requiredSpaceTotal))/float64(ui.TotalSpaceBytes) < 0.1 {
		return fmt.Errorf("Not enough space available in files repository, require: %d, available: %d, required headroom after download: 10%%", requiredSpaceTotal, ui.AvailableSpaceBytes)
	}

	return nil
}

func (s *updateService) UpdateConfig(ctx context.Context, updateFilterExpression string, updateFileFilterExpression string) {
	s.configUpdateMu.Lock()
	defer s.configUpdateMu.Unlock()

	s.updateFilterExpression = updateFilterExpression
	s.updateFileFilterExpression = updateFileFilterExpression
}
