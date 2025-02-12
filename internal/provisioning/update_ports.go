package provisioning

import (
	"context"
	"io"
)

type UpdateService interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllIDs(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id string) (Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, updateID string) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out repo/mock/update_repo_mock_gen.go -rm . UpdateRepo
//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i UpdateRepo -t ../logger/slog.gotmpl -o ./repo/middleware/update_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i UpdateRepo -t prometheus -o ./repo/middleware/update_prometheus_gen.go

type UpdateRepo interface {
	GetAll(ctx context.Context) (Updates, error)
	GetAllIDs(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id string) (Update, error)

	// Files
	GetUpdateAllFiles(ctx context.Context, updateID string) (UpdateFiles, error)
	GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error)
}
