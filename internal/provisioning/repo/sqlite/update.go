package sqlite

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/sqlite"
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

func (u update) AssignChannels(ctx context.Context, id uuid.UUID, channelNames []string) error {
	err := transaction.ForceTx(ctx, u.db, func(ctx context.Context, tx transaction.TX) error {
		update, err := entities.GetUpdate(ctx, tx, id)
		if err != nil {
			return err
		}

		return entities.UpdateUpdateChannels(ctx, tx, update.ID, channelNames)
	})
	if err != nil {
		return err
	}

	return nil
}

func (u update) GetAll(ctx context.Context) (provisioning.Updates, error) {
	var updates provisioning.Updates

	err := transaction.ForceTx(ctx, transaction.GetDBTX(ctx, u.db), func(ctx context.Context, tx transaction.TX) error {
		var err error
		updates, err = entities.GetUpdates(ctx, transaction.GetDBTX(ctx, u.db))
		if err != nil {
			return err
		}

		return u.enrichChannels(ctx, tx, updates)
	})
	if err != nil {
		return nil, err
	}

	return updates, nil
}

func (u update) GetAllWithFilter(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
	var updates provisioning.Updates

	err := transaction.ForceTx(ctx, transaction.GetDBTX(ctx, u.db), func(ctx context.Context, tx transaction.TX) error {
		var err error
		updates, err = entities.GetUpdates(ctx, transaction.GetDBTX(ctx, u.db), filter)
		if err != nil {
			return err
		}

		return u.enrichChannels(ctx, tx, updates)
	})
	if err != nil {
		return nil, err
	}

	return updates, nil
}

func (u update) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return entities.GetUpdateNames(ctx, transaction.GetDBTX(ctx, u.db))
}

func (u update) GetAllUUIDsWithFilter(ctx context.Context, filter provisioning.UpdateFilter) ([]uuid.UUID, error) {
	return entities.GetUpdateNames(ctx, transaction.GetDBTX(ctx, u.db), filter)
}

func (u update) GetByUUID(ctx context.Context, id uuid.UUID) (*provisioning.Update, error) {
	var update *provisioning.Update

	err := transaction.ForceTx(ctx, transaction.GetDBTX(ctx, u.db), func(ctx context.Context, tx transaction.TX) error {
		var err error

		update, err = entities.GetUpdate(ctx, tx, id)
		if err != nil {
			return err
		}

		updates := provisioning.Updates{*update}
		err = u.enrichChannels(ctx, tx, updates)
		*update = updates[0]
		return err
	})
	if err != nil {
		return nil, err
	}

	return update, nil
}

func (u update) enrichChannels(ctx context.Context, tx transaction.TX, updates provisioning.Updates) error {
	for i := range updates {
		channels, err := entities.GetUpdateChannels(ctx, tx, updates[i].ID)
		if err != nil {
			return err
		}

		channelNames := make([]string, 0, len(channels))
		for _, channel := range channels {
			channelNames = append(channelNames, channel.Name)
		}

		updates[i].Channels = channelNames
	}

	return nil
}

func (u update) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return entities.DeleteUpdate(ctx, transaction.GetDBTX(ctx, u.db), id)
}

func (u update) GetUpdatesByAssignedChannelName(ctx context.Context, name string, filter ...provisioning.UpdateFilter) (provisioning.Updates, error) {
	var updates provisioning.Updates

	err := transaction.ForceTx(ctx, transaction.GetDBTX(ctx, u.db), func(ctx context.Context, tx transaction.TX) error {
		var err error
		updates, err = entities.GetUpdatesByAssignedChannelName(ctx, transaction.GetDBTX(ctx, u.db), name, filter...)
		if err != nil {
			return err
		}

		return u.enrichChannels(ctx, tx, updates)
	})
	if err != nil {
		return nil, err
	}

	return updates, nil
}
