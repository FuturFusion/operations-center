package sqlite

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/internal/warning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
)

type warningDB struct {
	db sqlite.DBTX
}

func NewWarning(db sqlite.DBTX) warning.WarningRepo {
	return &warningDB{
		db: db,
	}
}

// DeleteByUUID implements warning.WarningRepo.
func (w *warningDB) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return entities.DeleteWarning(ctx, transaction.GetDBTX(ctx, w.db), id)
}

// GetAll implements warning.WarningRepo.
func (w *warningDB) GetAll(ctx context.Context) (warning.Warnings, error) {
	return entities.GetWarnings(ctx, transaction.GetDBTX(ctx, w.db))
}

// GetByUUID implements warning.WarningRepo.
func (w *warningDB) GetByUUID(ctx context.Context, id uuid.UUID) (*warning.Warning, error) {
	return entities.GetWarning(ctx, transaction.GetDBTX(ctx, w.db), id)
}

// GetByScopeAndType implements warning.WarningRepo.
func (w *warningDB) GetByScopeAndType(ctx context.Context, scope api.WarningScope, wType api.WarningType) (warning.Warnings, error) {
	if wType == "" || (scope.EntityType == "" && scope.Entity != "") {
		return nil, fmt.Errorf("Invalid scope. Requires warning type and entity type")
	}

	filter := warning.WarningFilter{Type: &wType}
	if scope.Scope != "" {
		filter.Scope = &scope.Scope
	}

	if scope.EntityType != "" {
		filter.EntityType = &scope.EntityType
		if scope.Entity != "" {
			filter.Entity = &scope.Entity
		}
	}

	warnings, err := entities.GetWarnings(ctx, transaction.GetDBTX(ctx, w.db), filter)
	if err != nil {
		return nil, err
	}

	return warnings, nil
}

// Update implements warning.WarningRepo.
func (w *warningDB) Update(ctx context.Context, id uuid.UUID, warn warning.Warning) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, w.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateWarning(ctx, tx, id, warn)
	})
}

// Upsert implements warning.WarningRepo.
func (w *warningDB) Upsert(ctx context.Context, warn warning.Warning) (int64, error) {
	return entities.CreateOrReplaceWarning(ctx, transaction.GetDBTX(ctx, w.db), warn)
}
