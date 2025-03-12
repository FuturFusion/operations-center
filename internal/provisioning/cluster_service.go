package provisioning

import (
	"context"
	"fmt"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type clusterService struct {
	repo             ClusterRepo
	serverSvc        ServerService
	inventorySyncers []InventorySyncer

	now func() time.Time
}

var _ ClusterService = &clusterService{}

type ClusterServiceOption func(s *clusterService)

func ClusterServiceWithNow(nowFunc func() time.Time) ClusterServiceOption {
	return func(s *clusterService) {
		s.now = nowFunc
	}
}

func NewClusterService(repo ClusterRepo, serverSvc ServerService, inventorySyncers []InventorySyncer, opts ...ClusterServiceOption) *clusterService {
	clusterSvc := &clusterService{
		repo:             repo,
		serverSvc:        serverSvc,
		inventorySyncers: inventorySyncers,

		now: time.Now,
	}

	for _, opt := range opts {
		opt(clusterSvc)
	}

	return clusterSvc
}

func (s *clusterService) SetInventorySyncers(inventorySyncers []InventorySyncer) {
	(*s).inventorySyncers = inventorySyncers
}

func (s clusterService) Create(ctx context.Context, newCluster Cluster) (Cluster, error) {
	err := newCluster.Validate()
	if err != nil {
		return Cluster{}, err
	}

	var cluster Cluster
	err = transaction.Do(ctx, func(ctx context.Context) error {
		var servers []Server
		for _, serverName := range newCluster.ServerNames {
			server, err := s.serverSvc.GetByName(ctx, serverName)
			if err != nil {
				return err
			}

			if server.Cluster != "" {
				return fmt.Errorf("Server %q is already part of cluster %q", serverName, server.Cluster)
			}

			servers = append(servers, server)
		}

		newCluster.LastUpdated = s.now()

		cluster, err = s.repo.Create(ctx, newCluster)
		if err != nil {
			return err
		}

		for _, server := range servers {
			server.Cluster = newCluster.Name
			_, err = s.serverSvc.UpdateByName(ctx, server.Name, server)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return Cluster{}, err
	}

	return cluster, nil
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

	newCluster.LastUpdated = s.now()

	return s.repo.UpdateByName(ctx, name, newCluster)
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
		cluster.LastUpdated = s.now()

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
