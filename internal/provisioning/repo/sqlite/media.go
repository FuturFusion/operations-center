package sqlite

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

type media struct {
	db sqlite.DBTX
}

var _ provisioning.MediaRepo = &media{}

func NewMedia(db sqlite.DBTX) *media {
	return &media{
		db: db,
	}
}

func (m media) Create(ctx context.Context, in provisioning.Media) (int64, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) (int64, error) {
		return entities.CreateMedia(ctx, transaction.GetDBTX(ctx, m.db), in)
	})
}

func (m media) GetAllWithFilter(ctx context.Context, filter provisioning.MediaFilter) (provisioning.Medias, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) (provisioning.Medias, error) {
		return entities.GetMedias(ctx, transaction.GetDBTX(ctx, m.db), filter)
	})
}

func (m media) GetByTypeAndName(ctx context.Context, mediaType provisioning.MediaType, name string) (*provisioning.Media, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) (*provisioning.Media, error) {
		return entities.GetMedia(ctx, transaction.GetDBTX(ctx, m.db), mediaType, name)
	})
}

func (m media) Update(ctx context.Context, in provisioning.Media) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, m.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateMedia(ctx, tx, in.Type, in.Name, in)
	})
}

func (m media) DeleteByTypeAndName(ctx context.Context, mediaType provisioning.MediaType, name string) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		return entities.DeleteMedia(ctx, transaction.GetDBTX(ctx, m.db), mediaType, name)
	})
}
