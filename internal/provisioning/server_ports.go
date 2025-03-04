package provisioning

import "context"

type ServerService interface {
	Create(ctx context.Context, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllByCluster(ctx context.Context, cluster string) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (Server, error)
	UpdateByName(ctx context.Context, name string, server Server) (Server, error)
	Rename(ctx context.Context, name string, to string) error
	DeleteByName(ctx context.Context, name string) error
}

type ServerRepo interface {
	Create(ctx context.Context, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllByCluster(ctx context.Context, cluster string) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (Server, error)
	UpdateByName(ctx context.Context, name string, server Server) (Server, error)
	Rename(ctx context.Context, name string, to string) error
	DeleteByName(ctx context.Context, name string) error
}
