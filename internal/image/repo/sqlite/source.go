package sqlite

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

type imageSource struct {
	db sqlite.DBTX
}

var _ image.IncusImageSourceRepo = &imageSource{}

func NewImageSource(db sqlite.DBTX) *imageSource {
	return &imageSource{
		db: db,
	}
}

func (i imageSource) Create(ctx context.Context, in image.IncusImageSource) (int64, error) {
	return entities.CreateIncusImageSource(ctx, transaction.GetDBTX(ctx, i.db), in)
}

func (i imageSource) GetAll(ctx context.Context) (image.IncusImageSources, error) {
	channels, err := entities.GetIncusImageSources(ctx, transaction.GetDBTX(ctx, i.db))
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func (i imageSource) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetIncusImageSourceNames(ctx, transaction.GetDBTX(ctx, i.db))
}

func (i imageSource) GetByName(ctx context.Context, name string) (*image.IncusImageSource, error) {
	channel, err := entities.GetIncusImageSource(ctx, transaction.GetDBTX(ctx, i.db), name)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

func (i imageSource) Update(ctx context.Context, in image.IncusImageSource) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, i.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateIncusImageSource(ctx, tx, in.Name, in)
	})
}

func (i imageSource) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteIncusImageSource(ctx, transaction.GetDBTX(ctx, i.db), name)
}
