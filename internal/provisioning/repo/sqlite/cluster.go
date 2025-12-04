package sqlite

import (
	"context"
	"errors"

	incustls "github.com/lxc/incus/v6/shared/tls"

	"github.com/FuturFusion/operations-center/internal/domain"
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
	clusters, err := entities.GetClusters(ctx, transaction.GetDBTX(ctx, c.db))
	if err != nil {
		return nil, err
	}

	var errs []error
	for i := range clusters {
		if clusters[i].Certificate == "" {
			continue
		}

		clusters[i].Fingerprint, err = incustls.CertFingerprintStr(clusters[i].Certificate)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return clusters, errors.Join(errs...)
}

func (c cluster) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetClusterNames(ctx, transaction.GetDBTX(ctx, c.db))
}

func (c cluster) GetByName(ctx context.Context, name string) (*provisioning.Cluster, error) {
	cluster, err := entities.GetCluster(ctx, transaction.GetDBTX(ctx, c.db), name)
	if err != nil {
		return nil, err
	}

	if cluster.Certificate == "" {
		return cluster, nil
	}

	cluster.Fingerprint, err = incustls.CertFingerprintStr(cluster.Certificate)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (c cluster) ExistsByName(ctx context.Context, name string) (bool, error) {
	_, err := entities.GetCluster(ctx, transaction.GetDBTX(ctx, c.db), name)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
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
