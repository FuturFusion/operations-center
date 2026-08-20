package cluster

import (
	"context"

	"github.com/lxc/incus/v7/shared/revert"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (s *clusterService) CheckClusteringServerConsistency(ctx context.Context, servers []provisioning.Server) (isConsistent bool, inconsistencyReason string, _ error) {
	return s.checkClusteringServerConsistency(ctx, servers)
}

func (s *clusterService) CopyServicesConfigFromClusterMember(ctx context.Context, sourceServer provisioning.Server, targetServers []provisioning.Server) error {
	reverter := revert.New()
	defer reverter.Fail()

	err := s.copyServicesConfigFromClusterMember(ctx, sourceServer, targetServers, reverter)
	if err != nil {
		return err
	}

	reverter.Success()

	return nil
}
