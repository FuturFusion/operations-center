package api

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

// joinCheck returns the check refusing to join a cluster while this server still manages servers or clusters, since the join discards them.
func (d *Daemon) joinCheck(serverSvc provisioning.ServerService, clusterSvc provisioning.ClusterService) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		servers, err := serverSvc.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get the servers before joining a cluster: %w", err)
		}

		managed := 0
		for _, server := range servers {
			if server.Type != api.ServerTypeOperationsCenter {
				managed++
			}
		}

		clusters, err := clusterSvc.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to get the clusters before joining a cluster: %w", err)
		}

		if managed == 0 && len(clusters) == 0 {
			return nil
		}

		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "This server manages %d servers and %d clusters, which joining a cluster discards", managed, len(clusters)).
			WithHintf("Remove the servers and clusters first, or force the join to discard them.")
	}
}
