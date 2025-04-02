package sqlite

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type cluster struct {
	db sqlite.DBTX
}

var _ provisioning.ClusterRepo = &cluster{}

func NewCluster(db sqlite.DBTX) *cluster {
	return &cluster{
		db: db,
	}
}

func (c cluster) Create(ctx context.Context, in provisioning.Cluster) (int64, error) {
	return entities.CreateCluster(ctx, transaction.GetDBTX(ctx, c.db), in)
}

func (c cluster) GetAll(ctx context.Context) (provisioning.Clusters, error) {
	return entities.GetClusters(ctx, transaction.GetDBTX(ctx, c.db))
}

func (c cluster) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetClusterNames(ctx, transaction.GetDBTX(ctx, c.db))
}

func (c cluster) GetByName(ctx context.Context, name string) (*provisioning.Cluster, error) {
	return entities.GetCluster(ctx, transaction.GetDBTX(ctx, c.db), name)
}

func (c cluster) Update(ctx context.Context, in provisioning.Cluster) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, c.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateCluster(ctx, tx, in.Name, in)
	})
}

func (c cluster) Rename(ctx context.Context, oldName string, newName string) error {
	return entities.RenameCluster(ctx, transaction.GetDBTX(ctx, c.db), oldName, newName)
}

func (c cluster) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteCluster(ctx, transaction.GetDBTX(ctx, c.db), name)
}
