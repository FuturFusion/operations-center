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

var _ image.SourceRepo = &imageSource{}

func NewImageSource(db sqlite.DBTX) *imageSource {
	return &imageSource{
		db: db,
	}
}

func (i imageSource) Create(ctx context.Context, in image.ImageSource) (int64, error) {
	return entities.CreateImageSource(ctx, transaction.GetDBTX(ctx, i.db), in)
}

func (i imageSource) GetAll(ctx context.Context) (image.Sources, error) {
	channels, err := entities.GetImageSources(ctx, transaction.GetDBTX(ctx, i.db))
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func (i imageSource) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetImageSourceNames(ctx, transaction.GetDBTX(ctx, i.db))
}

func (i imageSource) GetByName(ctx context.Context, name string) (*image.ImageSource, error) {
	channel, err := entities.GetImageSource(ctx, transaction.GetDBTX(ctx, i.db), name)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

func (i imageSource) Update(ctx context.Context, in image.ImageSource) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, i.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateImageSource(ctx, tx, in.Name, in)
	})
}

func (i imageSource) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteImageSource(ctx, transaction.GetDBTX(ctx, i.db), name)
}
