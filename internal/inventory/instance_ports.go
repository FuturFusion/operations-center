package inventory

import (
	"context"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type InstanceService interface {
	GetAllIDs(ctx context.Context) ([]int, error)
	GetByID(ctx context.Context, id int) (Instance, error)
	SyncAll(ctx context.Context) error
	SyncCluster(ctx context.Context, clusterID int) error
	SyncServer(ctx context.Context, serverID int) error
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out repo/mock/instance_repo_mock_gen.go -rm . InstanceRepo
//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i InstanceRepo -t ../logger/slog.gotmpl -o ./repo/middleware/instance_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i InstanceRepo -t prometheus -o ./repo/middleware/instance_prometheus_gen.go

type InstanceRepo interface {
	GetAllIDs(ctx context.Context) ([]int, error)
	GetByID(ctx context.Context, id int) (Instance, error)
	Create(ctx context.Context, instance Instance) (Instance, error)
	DeleteByServerID(ctx context.Context, serverID int) error
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out mock/cluster_service_mock_gen.go -rm . ClusterService

type ClusterService interface {
	GetAll(ctx context.Context) (provisioning.Clusters, error)
	GetByID(ctx context.Context, id int) (provisioning.Cluster, error)
}

// ServerClientProvider returns a ServerClient, which allows to query a server
// for its resources. E.g. query an Incus instance for its resources like
// instances, networks, storage volumes, etc.
type ServerClientProvider func(ctx context.Context, connectionURL string) (ServerClient, error)

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out mock/server_client_mock_gen.go -rm . ServerClient

type ServerClient interface {
	GetInstances(ctx context.Context) ([]incusapi.InstanceFull, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out mock/server_service_mock_gen.go -rm . ServerService

type ServerService interface {
	GetAllByClusterID(ctx context.Context, clusterID int) (provisioning.Servers, error)
	GetByID(ctx context.Context, id int) (provisioning.Server, error)
}
