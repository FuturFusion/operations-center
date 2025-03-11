package provisioning

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type clusterService struct {
	repo             ClusterRepo
	inventorySyncers []InventorySyncer
}

var _ ClusterService = &clusterService{}

func NewClusterService(repo ClusterRepo, inventorySyncers []InventorySyncer) *clusterService {
	return &clusterService{
		repo:             repo,
		inventorySyncers: inventorySyncers,
	}
}

func (s *clusterService) SetInventorySyncers(inventorySyncers []InventorySyncer) {
	(*s).inventorySyncers = inventorySyncers
}

func (s clusterService) Create(ctx context.Context, newCluster Cluster) (Cluster, error) {
	err := newCluster.Validate()
	if err != nil {
		return Cluster{}, err
	}

	return s.repo.Create(ctx, newCluster)
}

func (s clusterService) GetAll(ctx context.Context) (Clusters, error) {
	return s.repo.GetAll(ctx)
}

func (s clusterService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s clusterService) GetByName(ctx context.Context, name string) (Cluster, error) {
	if name == "" {
		return Cluster{}, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s clusterService) UpdateByName(ctx context.Context, name string, newCluster Cluster) (Cluster, error) {
	if name == "" {
		return Cluster{}, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := newCluster.Validate()
	if err != nil {
		return Cluster{}, err
	}

	if name != newCluster.Name {
		return Cluster{}, domain.NewValidationErrf("Invalid cluster, name mismatch")
	}

	var cluster Cluster
	err = transaction.Do(ctx, func(ctx context.Context) error {
		cluster, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return err
		}

		if len(newCluster.ServerHostnames) != len(cluster.ServerHostnames) {
			return fmt.Errorf("Clusters can not be shrunk or grown: %w", domain.ErrConstraintViolation)
		}

		cluster, err = s.repo.UpdateByName(ctx, name, newCluster)
		return err
	})
	if err != nil {
		return Cluster{}, err
	}

	return cluster, nil
}

func (s clusterService) RenameByName(ctx context.Context, name string, newCluster Cluster) (Cluster, error) {
	if name == "" {
		return Cluster{}, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newCluster.Name == "" {
		return Cluster{}, domain.NewValidationErrf("Invalid cluster, name cannot by empty")
	}

	var cluster Cluster
	var err error
	err = transaction.Do(ctx, func(ctx context.Context) error {
		cluster, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return err
		}

		cluster.Name = newCluster.Name

		cluster, err = s.repo.UpdateByName(ctx, name, cluster)
		return err
	})
	if err != nil {
		return Cluster{}, err
	}

	return cluster, nil
}

func (s clusterService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// FIXME: deleteting a server also requires to delete all the inventory (in a transaction).

	return s.repo.DeleteByName(ctx, name)
}

func (s clusterService) ResyncInventoryByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	for _, inventorySyncer := range s.inventorySyncers {
		err := inventorySyncer.SyncCluster(ctx, name)
		if err != nil {
			return err
		}
	}

	return nil
}
