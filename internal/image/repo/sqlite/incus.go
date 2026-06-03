package sqlite

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

type incusImage struct {
	db sqlite.DBTX
}

var _ image.ImageIncusRepo = &incusImage{}

func NewIncusImage(db sqlite.DBTX) *incusImage {
	return &incusImage{
		db: db,
	}
}

func (i incusImage) Create(ctx context.Context, in image.IncusImage) (int64, error) {
	return entities.CreateIncusImage(ctx, transaction.GetDBTX(ctx, i.db), in)
}

func (i incusImage) GetAll(ctx context.Context) (image.IncusImages, error) {
	channels, err := entities.GetIncusImages(ctx, transaction.GetDBTX(ctx, i.db))
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func (i incusImage) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetIncusImageNames(ctx, transaction.GetDBTX(ctx, i.db))
}

func (i incusImage) GetByName(ctx context.Context, name string) (*image.IncusImage, error) {
	channel, err := entities.GetIncusImage(ctx, transaction.GetDBTX(ctx, i.db), name)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

func (i incusImage) Update(ctx context.Context, in image.IncusImage) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, i.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateIncusImage(ctx, tx, in.Name, in)
	})
}

func (i incusImage) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteIncusImage(ctx, transaction.GetDBTX(ctx, i.db), name)
}
