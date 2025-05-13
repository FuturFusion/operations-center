package sqlite

import (
	"context"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type update struct {
	db sqlite.DBTX
}

var _ provisioning.UpdateRepo = &update{}

func NewUpdate(db sqlite.DBTX) *update {
	return &update{
		db: db,
	}
}

func (u update) Create(ctx context.Context, in provisioning.Update) (int64, error) {
	return entities.CreateUpdate(ctx, transaction.GetDBTX(ctx, u.db), in)
}

func (u update) GetAll(ctx context.Context) (provisioning.Updates, error) {
	return entities.GetUpdates(ctx, transaction.GetDBTX(ctx, u.db))
}

func (u update) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return entities.GetUpdateNames(ctx, transaction.GetDBTX(ctx, u.db))
}

func (u update) GetByUUID(ctx context.Context, id uuid.UUID) (*provisioning.Update, error) {
	return entities.GetUpdate(ctx, transaction.GetDBTX(ctx, u.db), id)
}

func (u update) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return entities.DeleteUpdate(ctx, transaction.GetDBTX(ctx, u.db), id)
}
