package provisioning

import (
	"context"
)

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
