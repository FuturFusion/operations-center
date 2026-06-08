package image

import (
	"context"
	"io"
	"mime/multipart"
)

type ImageIncusService interface {
	AddVersion(ctx context.Context, name string, version string, mr *multipart.Reader) error
	GetAll(ctx context.Context) (IncusImages, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*IncusImage, error)
	DeleteByName(ctx context.Context, name string) error
	DeleteVersionByName(ctx context.Context, name string, version string) error
	DeleteBySource(ctx context.Context, sourceName string) error
	GetVersionFileByName(ctx context.Context, name string, version string, filename string) (_ io.ReadCloser, size int64, _ error)
	Update(ctx context.Context, incusImage IncusImage) error
	ValidateFilterExpression(ctx context.Context, filterExpression string) error
	RefreshFromSource(ctx context.Context, source ImageSource) error
}

type ImageIncusRepo interface {
	Create(ctx context.Context, newIncusImage IncusImage) (int64, error)
	GetAll(ctx context.Context) (IncusImages, error)
	GetAllWithFilter(ctx context.Context, filter IncusImageFilter) (IncusImages, error)
	GetAllNames(ctx context.Context) ([]string, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
	GetByName(ctx context.Context, name string) (*IncusImage, error)
	Update(ctx context.Context, incusImage IncusImage) error
	Upsert(ctx context.Context, incusImage IncusImage) error
	DeleteByName(ctx context.Context, name string) error
}

type (
	CommitFunc func() error
	CancelFunc func() error
)

type ImageIncusFileRepo interface {
	Exists(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) (bool, error)
	Get(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) (_ io.ReadCloser, size int64, _ error)
	Put(ctx context.Context, img *IncusImage, versionIdentifier string, filename string, content io.ReadCloser) (_ CommitFunc, _ CancelFunc, size int64, _ error)
	Delete(ctx context.Context, img *IncusImage) error
	DeleteVersion(ctx context.Context, img *IncusImage, versionIdentifier string) error
	DeleteVersionFile(ctx context.Context, img *IncusImage, versionIdentifier string, filename string) error
	UsageInformation(ctx context.Context) (UsageInformation, error)
}

type SimplestreamsPort interface {
	GetImageList(ctx context.Context, source ImageSource) (IncusImages, error)
	GetFile(ctx context.Context, source ImageSource, path string) (io.ReadCloser, error)
}
