package sqlite

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

func (u update) CreateExposedchannel(ctx context.Context, in provisioning.Exposedchannel) (int64, error) {
	return entities.CreateExposedchannel(ctx, transaction.GetDBTX(ctx, u.db), in)
}

func (u update) GetAllExposedchannels(ctx context.Context) (provisioning.Exposedchannels, error) {
	exposedchannels, err := entities.GetExposedchannels(ctx, transaction.GetDBTX(ctx, u.db))
	if err != nil {
		return nil, err
	}

	return exposedchannels, nil
}

func (u update) GetAllExposedchannelNames(ctx context.Context) ([]string, error) {
	return entities.GetExposedchannelNames(ctx, transaction.GetDBTX(ctx, u.db))
}

func (u update) GetExposedchannelByName(ctx context.Context, name string) (*provisioning.Exposedchannel, error) {
	exposedchannel, err := entities.GetExposedchannel(ctx, transaction.GetDBTX(ctx, u.db), name)
	if err != nil {
		return nil, err
	}

	return exposedchannel, nil
}

func (u update) UpdateExposedchannel(ctx context.Context, in provisioning.Exposedchannel) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, u.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateExposedchannel(ctx, tx, in.Name, in)
	})
}

func (u update) RenameExposedchannel(ctx context.Context, oldName string, newName string) error {
	return entities.RenameExposedchannel(ctx, transaction.GetDBTX(ctx, u.db), oldName, newName)
}

func (u update) DeleteExposedchannelByName(ctx context.Context, name string) error {
	return entities.DeleteExposedchannel(ctx, transaction.GetDBTX(ctx, u.db), name)
}
