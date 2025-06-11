package provisioning

import (
	"context"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type ServerService interface {
	Create(ctx context.Context, token uuid.UUID, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllWithFilter(ctx context.Context, filter ServerFilter) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetAllNamesWithFilter(ctx context.Context, filter ServerFilter) ([]string, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	Update(ctx context.Context, server Server) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error

	PollPendingServers(ctx context.Context) error
}

type ServerRepo interface {
	Create(ctx context.Context, server Server) (int64, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllWithFilter(ctx context.Context, filter ServerFilter) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetAllNamesWithFilter(ctx context.Context, filter ServerFilter) ([]string, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	Update(ctx context.Context, server Server) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
}

type ServerClientPort interface {
	Ping(ctx context.Context, server Server) error
	GetResources(ctx context.Context, server Server) (api.HardwareData, error)
	GetOSData(ctx context.Context, server Server) (api.OSData, error)
}
