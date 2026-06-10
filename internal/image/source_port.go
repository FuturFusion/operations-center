package image

import (
	"context"
)

type SourceService interface {
	Create(ctx context.Context, source ImageSource) (ImageSource, error)
	GetAll(ctx context.Context) (Sources, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*ImageSource, error)
	Update(ctx context.Context, source ImageSource) error
	DeleteByName(ctx context.Context, name string) error
	RefreshAll(ctx context.Context) error
	RefreshByName(ctx context.Context, name string) error
}

type SourceRepo interface {
	Create(ctx context.Context, source ImageSource) (int64, error)
	GetAll(ctx context.Context) (Sources, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*ImageSource, error)
	Update(ctx context.Context, source ImageSource) error
	DeleteByName(ctx context.Context, name string) error
}

type ImageSourcerPort interface {
	ValidateFilterExpression(ctx context.Context, filterExpression string) error
	RefreshFromSource(ctx context.Context, source ImageSource) error
	DeleteBySource(ctx context.Context, sourceName string) error
}
