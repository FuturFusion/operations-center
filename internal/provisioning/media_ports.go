package provisioning

import (
	"context"
)

// MediaRepo records the generated media and the cluster member storing them.
type MediaRepo interface {
	Create(ctx context.Context, newMedia Media) (int64, error)
	GetAllWithFilter(ctx context.Context, filter MediaFilter) (Medias, error)
	GetByTypeAndName(ctx context.Context, mediaType MediaType, name string) (*Media, error)
	Update(ctx context.Context, media Media) error
	DeleteByTypeAndName(ctx context.Context, mediaType MediaType, name string) error
}
