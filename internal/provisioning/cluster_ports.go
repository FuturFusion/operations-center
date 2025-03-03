package provisioning

import (
	"context"
)

//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ClusterService -t ../logger/slog.gotmpl -o ./middleware/cluster_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ClusterService -t prometheus -o ./middleware/cluster_prometheus_gen.go

type ClusterService interface {
	Create(ctx context.Context, cluster Cluster) (Cluster, error)
	GetAll(ctx context.Context) (Clusters, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id int) (Cluster, error)
	GetByName(ctx context.Context, name string) (Cluster, error)
	UpdateByName(ctx context.Context, name string, cluster Cluster) (Cluster, error)
	RenameByName(ctx context.Context, name string, cluster Cluster) (Cluster, error)
	DeleteByName(ctx context.Context, name string) error
	ResyncInventoryByName(ctx context.Context, name string) error
}

//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ClusterRepo -t ../logger/slog.gotmpl -o ./repo/middleware/cluster_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ClusterRepo -t prometheus -o ./repo/middleware/cluster_prometheus_gen.go

type ClusterRepo interface {
	Create(ctx context.Context, cluster Cluster) (Cluster, error)
	GetAll(ctx context.Context) (Clusters, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id int) (Cluster, error)
	GetByName(ctx context.Context, name string) (Cluster, error)
	UpdateByID(ctx context.Context, cluster Cluster) (Cluster, error)
	DeleteByID(ctx context.Context, id int) error
}

type InventorySyncer interface {
	SyncCluster(ctx context.Context, clusterID int) error
}
