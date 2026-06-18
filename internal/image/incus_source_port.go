package image

import (
	"context"
)

type IncusImageSourceService interface {
	Create(ctx context.Context, source IncusImageSource) (IncusImageSource, error)
	GetAll(ctx context.Context) (IncusImageSources, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*IncusImageSource, error)
	Update(ctx context.Context, source IncusImageSource) error
	DeleteByName(ctx context.Context, name string) error
	RefreshAll(ctx context.Context) error
	RefreshByName(ctx context.Context, name string) error
}

type IncusImageSourceRepo interface {
	Create(ctx context.Context, source IncusImageSource) (int64, error)
	GetAll(ctx context.Context) (IncusImageSources, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*IncusImageSource, error)
	Update(ctx context.Context, source IncusImageSource) error
	DeleteByName(ctx context.Context, name string) error
}

type IncusImageSourcePort interface {
	ValidateFilterExpression(ctx context.Context, filterExpression string) error
	RefreshFromSource(ctx context.Context, source IncusImageSource) error
	DeleteBySource(ctx context.Context, sourceName string) error
}
