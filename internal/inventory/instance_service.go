package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type instanceService struct {
	repo                   InstanceRepo
	clusterSvc             ClusterService
	serverSvc              ServerService
	instanceClientProvider ServerClientProvider

	now func() time.Time
}

var _ InstanceService = &instanceService{}

type InstanceServiceOption func(s *instanceService)

func NewInstanceService(repo InstanceRepo, clusterSvc ClusterService, serverSvc ServerService, instanceClientProvider ServerClientProvider, opts ...InstanceServiceOption) instanceService {
	instanceSvc := instanceService{
		repo:                   repo,
		clusterSvc:             clusterSvc,
		serverSvc:              serverSvc,
		instanceClientProvider: instanceClientProvider,

		now: time.Now,
	}

	for _, opt := range opts {
		opt(&instanceSvc)
	}

	return instanceSvc
}

func (s instanceService) GetAllIDs(ctx context.Context) ([]int, error) {
	return s.repo.GetAllIDs(ctx)
}

func (s instanceService) GetByID(ctx context.Context, id int) (Instance, error) {
	return s.repo.GetByID(ctx, id)
}

func (s instanceService) SyncAll(ctx context.Context) error {
	clusters, err := s.clusterSvc.GetAll(ctx)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		err = s.SyncCluster(ctx, cluster.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s instanceService) SyncCluster(ctx context.Context, clusterID int) error {
	servers, err := s.serverSvc.GetAllByClusterID(ctx, clusterID)
	if err != nil {
		return err
	}

	for _, server := range servers {
		err = s.SyncServer(ctx, server.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s instanceService) SyncServer(ctx context.Context, serverID int) error {
	server, err := s.serverSvc.GetByID(ctx, serverID)
	if err != nil {
		return err
	}

	client, err := s.instanceClientProvider(ctx, server.ConnectionURL)
	if err != nil {
		return err
	}

	serverInstances, err := client.GetInstances(ctx)
	if err != nil {
		return err
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		err = s.repo.DeleteByServerID(ctx, serverID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		for _, serverInstance := range serverInstances {
			instance := Instance{
				ClusterID:   server.ClusterID,
				ServerID:    serverID,
				ProjectName: serverInstance.Project,
				Name:        serverInstance.Name,
				Object:      serverInstance,
				LastUpdated: s.now(),
			}

			_, err := s.repo.Create(ctx, instance)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
