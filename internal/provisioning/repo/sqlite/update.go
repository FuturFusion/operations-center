package sqlite

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
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

func (u update) Upsert(ctx context.Context, in provisioning.Update) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, u.db), func(ctx context.Context, tx transaction.TX) error {
		_, err := entities.GetUpdate(ctx, transaction.GetDBTX(ctx, u.db), in.UUID)
		if errors.Is(err, domain.ErrNotFound) {
			_, err = entities.CreateUpdate(ctx, transaction.GetDBTX(ctx, u.db), in)
			if err != nil {
				return err
			}
		}

		if err != nil {
			return err
		}

		err = entities.UpdateUpdate(ctx, tx, in.UUID, in)
		if err != nil {
			return err
		}

		return nil
	})
}

func (u update) GetAll(ctx context.Context) (provisioning.Updates, error) {
	return entities.GetUpdates(ctx, transaction.GetDBTX(ctx, u.db))
}

func (u update) GetAllWithFilter(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
	return entities.GetUpdates(ctx, transaction.GetDBTX(ctx, u.db), filter)
}

func (u update) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return entities.GetUpdateNames(ctx, transaction.GetDBTX(ctx, u.db))
}

func (u update) GetAllUUIDsWithFilter(ctx context.Context, filter provisioning.UpdateFilter) ([]uuid.UUID, error) {
	return entities.GetUpdateNames(ctx, transaction.GetDBTX(ctx, u.db), filter)
}

func (u update) GetByUUID(ctx context.Context, id uuid.UUID) (*provisioning.Update, error) {
	return entities.GetUpdate(ctx, transaction.GetDBTX(ctx, u.db), id)
}

func (u update) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return entities.DeleteUpdate(ctx, transaction.GetDBTX(ctx, u.db), id)
}

func (u update) AssignExposedchannel(ctx context.Context, UUID uuid.UUID, exposedchannel string) error {
	return transaction.ForceTx(ctx, u.db, func(ctx context.Context, tx transaction.TX) error {
		updateID, err := entities.GetUpdateID(ctx, tx, UUID)
		if err != nil {
			return err
		}

		exposedchannelID, err := entities.GetExposedchannelID(ctx, tx, exposedchannel)
		if err != nil {
			return err
		}

		return entities.CreateUpdateExposedchannels(ctx, tx, []provisioning.ExposedchannelUpdate{
			{
				UpdateID:         int(updateID),
				ExposedchannelID: int(exposedchannelID),
			},
		})
	})
}

func (u update) GetUpdatesByAssignedExposedchannelName(ctx context.Context, name string) (provisioning.Updates, error) {
	return entities.GetUpdatesByAssignedExposedchannelName(ctx, transaction.GetDBTX(ctx, u.db), name)
}
