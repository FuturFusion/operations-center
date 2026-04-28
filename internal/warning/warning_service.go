package warning

// TODO: Decide, if warning.Emit should be globally available (without dependency injection) or not?
// TODO: Should a similar pattern be applied as with slog.Default(), where a global instance can be overwritten?
// TODO: Should warning.Emit also take care of logging, such that this is not required in the business logic?

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type warningService struct {
	repo WarningRepo

	now func() time.Time
}

var _ WarningService = &warningService{}

type WarningServiceOption func(s *warningService)

func WithWarningServiceNow(nowFunc func() time.Time) WarningServiceOption {
	return func(s *warningService) {
		s.now = nowFunc
	}
}

func NewWarningService(repo WarningRepo, opts ...WarningServiceOption) WarningService {
	warningSvc := &warningService{
		repo: repo,

		now: time.Now,
	}

	for _, opt := range opts {
		opt(warningSvc)
	}

	return warningSvc
}

// DeleteByUUID implements WarningService.
func (w warningService) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	err := w.repo.DeleteByUUID(ctx, id)
	if err != nil {
		return err
	}

	return nil
}

// Emit records the given warning. If another warning of the same scope and type already exists,
// their messages and count will be merged, with new messages appearing at the end of the list.
func (w warningService) Emit(ctx context.Context, warning Warning) {
	var err error
	defer func() {
		if err != nil {
			slog.WarnContext(ctx, "Failed to record warning", slog.String("uuid", warning.UUID.String()), logger.Err(err))
		}
	}()

	err = warning.Validate()
	if err != nil {
		return
	}

	slog.WarnContext(ctx,
		strings.Join(warning.Messages, "; "), //nolint:sloglint
		slog.String("uuid", warning.UUID.String()),
		slog.String("type", string(warning.Type)),
		slog.String("scope", warning.Scope),
		slog.String("entity_type", warning.EntityType),
		slog.String("entity", warning.Entity),
	)

	err = transaction.Do(ctx, func(ctx context.Context) error {
		scope := api.WarningScope{Scope: warning.Scope, EntityType: warning.EntityType, Entity: warning.Entity}
		dbWarnings, err := w.repo.GetByScopeAndType(ctx, scope, warning.Type)
		if err != nil {
			return err
		}

		if len(dbWarnings) > 1 {
			return fmt.Errorf("Invalid warning state for scope %v", scope)
		}

		// If the warning already exists, re-use it and increment its count.
		warning.FirstOccurrence = w.now()
		warning.LastOccurrence = w.now()
		warning.LastUpdated = w.now()

		if len(dbWarnings) == 1 {
			dbWarning := dbWarnings[0]
			warning.UUID = dbWarning.UUID
			warning.Count += dbWarning.Count
			warning.FirstOccurrence = dbWarning.FirstOccurrence
			newMessages := make([]string, 0, len(dbWarning.Messages)+len(warning.Messages))
			for _, msg := range dbWarning.Messages {
				if !slices.Contains(warning.Messages, msg) {
					newMessages = append(newMessages, msg)
				}
			}

			// Append new messages at the end.
			newMessages = append(newMessages, warning.Messages...)
			warning.Messages = newMessages
		}

		_, err = w.repo.Upsert(ctx, warning)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return
	}
}

// GetAll implements WarningService.
func (w warningService) GetAll(ctx context.Context) (Warnings, error) {
	warnings, err := w.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	return warnings, nil
}

// GetByScopeAndType implements WarningService.
func (w warningService) GetByScopeAndType(ctx context.Context, scope api.WarningScope, wType api.WarningType) (Warnings, error) {
	warnings, err := w.repo.GetByScopeAndType(ctx, scope, wType)
	if err != nil {
		return nil, err
	}

	return warnings, nil
}

// GetByUUID implements WarningService.
func (w warningService) GetByUUID(ctx context.Context, id uuid.UUID) (*Warning, error) {
	warnings, err := w.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, err
	}

	return warnings, nil
}

// RemoveStale prunes all warning messages in the scope which are different from the provided set.
// Only duplicates of the provided set, or warnings of other scopes will remain.
func (w warningService) RemoveStale(ctx context.Context, scope api.WarningScope, newWarnings Warnings) {
	var err error
	defer func() {
		if err != nil {
			slog.WarnContext(ctx, "Failed to remove stale warnings",
				slog.String("scope", scope.Scope),
				slog.String("entity_type", scope.EntityType),
				slog.String("entity", scope.Entity),
				logger.Err(err),
			)
		}
	}()

	messagesByType := map[api.WarningType]map[string]bool{}
	for _, w := range newWarnings {
		if !w.Match(scope) {
			continue
		}

		if messagesByType[w.Type] == nil {
			messagesByType[w.Type] = map[string]bool{}
		}

		for _, msg := range w.Messages {
			messagesByType[w.Type][msg] = true
		}
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		allWarnings, err := w.repo.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get all warnings: %w", err)
		}

		for _, warning := range allWarnings {
			if !warning.Match(scope) {
				continue
			}

			seenMessages := make([]string, 0, len(warning.Messages))
			for _, msg := range warning.Messages {
				if messagesByType[warning.Type][msg] {
					seenMessages = append(seenMessages, msg)
				}
			}

			// All messages for this warning are stale, so delete it.
			if len(seenMessages) == 0 {
				err := w.repo.DeleteByUUID(ctx, warning.UUID)
				if err != nil {
					return fmt.Errorf("Failed to delete stale warning: %w", err)
				}

				continue
			}

			if slices.Equal(warning.Messages, seenMessages) {
				continue
			}

			warning.Messages = seenMessages
			err := w.repo.Update(ctx, warning.UUID, warning)
			if err != nil {
				return fmt.Errorf("Failed to prune stale warning messages: %w", err)
			}
		}

		return nil
	})
}

// UpdateStatusByUUID implements WarningService.
func (w warningService) UpdateStatusByUUID(ctx context.Context, id uuid.UUID, status api.WarningStatus) (*Warning, error) {
	var warning Warning
	err := transaction.Do(ctx, func(ctx context.Context) error {
		var err error
		dbWarning, err := w.repo.GetByUUID(ctx, id)
		if err != nil {
			return err
		}

		warning = *dbWarning

		warning.Status = status
		warning.LastUpdated = w.now()

		return w.repo.Update(ctx, warning.UUID, warning)
	})
	if err != nil {
		return nil, err
	}

	return &warning, nil
}
