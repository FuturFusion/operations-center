package provisioning

import (
	"context"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type ServerService interface {
	SetClusterService(clusterSvc ClusterService)
	Create(ctx context.Context, token uuid.UUID, server Server) (Server, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllWithFilter(ctx context.Context, filter ServerFilter) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetAllNamesWithFilter(ctx context.Context, filter ServerFilter) ([]string, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	Update(ctx context.Context, server Server) error
	UpdateSystemNetwork(ctx context.Context, name string, networkConfig ServerSystemNetwork) error
	SelfUpdate(ctx context.Context, serverUpdate ServerSelfUpdate) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error

	PollServers(ctx context.Context, serverStatus api.ServerStatus, updateServerConfiguration bool) error
}

type ServerRepo interface {
	Create(ctx context.Context, server Server) (int64, error)
	GetAll(ctx context.Context) (Servers, error)
	GetAllWithFilter(ctx context.Context, filter ServerFilter) (Servers, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetAllNamesWithFilter(ctx context.Context, filter ServerFilter) ([]string, error)
	GetByName(ctx context.Context, name string) (*Server, error)
	GetByCertificate(ctx context.Context, certificatePEM string) (*Server, error)
	Update(ctx context.Context, server Server) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
}

type ServerClientPort interface {
	Ping(ctx context.Context, endpoint Endpoint) error
	GetResources(ctx context.Context, endpoint Endpoint) (api.HardwareData, error)
	GetOSData(ctx context.Context, endpoint Endpoint) (api.OSData, error)
	GetServerType(ctx context.Context, endpoint Endpoint) (api.ServerType, error)
	UpdateNetworkConfig(ctx context.Context, server Server) error
}
