package provisioning

import "context"

func (s *clusterService) CheckClusteringServerConsistency(ctx context.Context, servers []Server) (isConsistent bool, inconsistencyReason string, _ error) {
	return s.checkClusteringServerConsistency(ctx, servers)
}
