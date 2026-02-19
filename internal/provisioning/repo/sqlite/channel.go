package sqlite

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

type channel struct {
	db sqlite.DBTX
}

var _ provisioning.ChannelRepo = &channel{}

func NewChannel(db sqlite.DBTX) *channel {
	return &channel{
		db: db,
	}
}

func (c channel) Create(ctx context.Context, in provisioning.Channel) (int64, error) {
	return entities.CreateChannel(ctx, transaction.GetDBTX(ctx, c.db), in)
}

func (c channel) GetAll(ctx context.Context) (provisioning.Channels, error) {
	channels, err := entities.GetChannels(ctx, transaction.GetDBTX(ctx, c.db))
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func (c channel) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetChannelNames(ctx, transaction.GetDBTX(ctx, c.db))
}

func (c channel) GetByName(ctx context.Context, name string) (*provisioning.Channel, error) {
	channel, err := entities.GetChannel(ctx, transaction.GetDBTX(ctx, c.db), name)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

func (c channel) Update(ctx context.Context, in provisioning.Channel) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, c.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateChannel(ctx, tx, in.Name, in)
	})
}

func (c channel) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteChannel(ctx, transaction.GetDBTX(ctx, c.db), name)
}
