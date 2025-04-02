package provisioning

import "context"

type ServerService interface {
	Create(ctx context.Context, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	Update(ctx context.Context, server Server) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
}

type ServerRepo interface {
	Create(ctx context.Context, server Server) (int64, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	Update(ctx context.Context, server Server) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
}
