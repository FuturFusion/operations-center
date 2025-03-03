package provisioning

import "context"

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
