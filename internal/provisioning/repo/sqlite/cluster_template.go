package sqlite

import (
	"context"
	"errors"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type clusterTemplate struct {
	db sqlite.DBTX
}

var _ provisioning.ClusterTemplateRepo = &clusterTemplate{}

func NewClusterTemplate(db sqlite.DBTX) *clusterTemplate {
	return &clusterTemplate{
		db: db,
	}
}

func (c clusterTemplate) Create(ctx context.Context, in provisioning.ClusterTemplate) (int64, error) {
	return entities.CreateClusterTemplate(ctx, transaction.GetDBTX(ctx, c.db), in)
}

func (c clusterTemplate) GetAll(ctx context.Context) (provisioning.ClusterTemplates, error) {
	return entities.GetClusterTemplates(ctx, transaction.GetDBTX(ctx, c.db))
}

func (c clusterTemplate) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetClusterTemplateNames(ctx, transaction.GetDBTX(ctx, c.db))
}

func (c clusterTemplate) GetByName(ctx context.Context, name string) (*provisioning.ClusterTemplate, error) {
	return entities.GetClusterTemplate(ctx, transaction.GetDBTX(ctx, c.db), name)
}

func (c clusterTemplate) ExistsByName(ctx context.Context, name string) (bool, error) {
	_, err := entities.GetClusterTemplate(ctx, transaction.GetDBTX(ctx, c.db), name)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

func (c clusterTemplate) Update(ctx context.Context, in provisioning.ClusterTemplate) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, c.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateClusterTemplate(ctx, tx, in.Name, in)
	})
}

func (c clusterTemplate) Rename(ctx context.Context, oldName string, newName string) error {
	return entities.RenameClusterTemplate(ctx, transaction.GetDBTX(ctx, c.db), oldName, newName)
}

func (c clusterTemplate) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteClusterTemplate(ctx, transaction.GetDBTX(ctx, c.db), name)
}
