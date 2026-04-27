package cluster

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (s *clusterService) CheckClusteringServerConsistency(ctx context.Context, servers []provisioning.Server) (isConsistent bool, inconsistencyReason string, _ error) {
	return s.checkClusteringServerConsistency(ctx, servers)
}
