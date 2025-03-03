package provisioning

import "context"

//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ServerService -t ../logger/slog.gotmpl -o ./middleware/server_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ServerService -t prometheus -o ./middleware/server_prometheus_gen.go

type ServerService interface {
	Create(ctx context.Context, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllByClusterID(ctx context.Context, clusterID int) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id int) (Server, error)
	GetByName(ctx context.Context, name string) (Server, error)
	UpdateByName(ctx context.Context, name string, server Server) (Server, error)
	RenameByName(ctx context.Context, name string, server Server) (Server, error)
	DeleteByName(ctx context.Context, name string) error
}

//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ServerRepo -t ../logger/slog.gotmpl -o ./repo/middleware/server_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ServerRepo -t prometheus -o ./repo/middleware/server_prometheus_gen.go

type ServerRepo interface {
	Create(ctx context.Context, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllByClusterID(ctx context.Context, clusterID int) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id int) (Server, error)
	GetByName(ctx context.Context, name string) (Server, error)
	UpdateByID(ctx context.Context, server Server) (Server, error)
	DeleteByID(ctx context.Context, id int) error
}
