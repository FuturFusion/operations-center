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

	err = transaction.Do(ctx, func(ctx context.Context) error {
		var servers []Server
		for _, serverName := range newCluster.ServerNames {
			server, err := s.serverSvc.GetByName(ctx, serverName)
			if err != nil {
				return err
			}

			if server.Cluster != nil {
				return fmt.Errorf("Server %q is already part of cluster %q", serverName, *server.Cluster)
			}

			servers = append(servers, *server)
		}

		newCluster.LastUpdated = s.now()

		newCluster.ID, err = s.repo.Create(ctx, newCluster)
		if err != nil {
			return err
		}

		for _, server := range servers {
			server.Cluster = &newCluster.Name
			err = s.serverSvc.Update(ctx, server)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return Cluster{}, err
	}

	return newCluster, nil
}

func (s clusterService) GetAll(ctx context.Context) (Clusters, error) {
	return s.repo.GetAll(ctx)
}

func (s clusterService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s clusterService) GetByName(ctx context.Context, name string) (*Cluster, error) {
	if name == "" {
		return nil, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s clusterService) Update(ctx context.Context, newCluster Cluster) error {
	err := newCluster.Validate()
	if err != nil {
		return err
	}

	newCluster.LastUpdated = s.now()

	return s.repo.Update(ctx, newCluster)
}

func (s clusterService) Rename(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New Cluster name cannot by empty")
	}

	return s.repo.Rename(ctx, oldName, newName)
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
