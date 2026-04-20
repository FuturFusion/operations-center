package warning

import (
	"context"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type WarningService interface {
	GetAll(ctx context.Context) (Warnings, error)
	GetByScopeAndType(ctx context.Context, scope api.WarningScope, wType api.WarningType) (Warnings, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Warning, error)
	UpdateStatusByUUID(ctx context.Context, id uuid.UUID, status api.WarningStatus) (*Warning, error)
	DeleteByUUID(ctx context.Context, id uuid.UUID) error

	Emit(ctx context.Context, w Warning)
	RemoveStale(ctx context.Context, scope api.WarningScope, newWarnings Warnings)
}

type WarningRepo interface {
	Upsert(ctx context.Context, w Warning) (int64, error)
	GetAll(ctx context.Context) (Warnings, error)
	GetByScopeAndType(ctx context.Context, scope api.WarningScope, wType api.WarningType) (Warnings, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Warning, error)
	Update(ctx context.Context, id uuid.UUID, w Warning) error
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
}
