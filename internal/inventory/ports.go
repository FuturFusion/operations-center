package inventory

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type ClusterService interface {
	GetAll(ctx context.Context) (provisioning.Clusters, error)
	GetByID(ctx context.Context, id int) (provisioning.Cluster, error)
}

type ServerService interface {
	GetAllByClusterID(ctx context.Context, clusterID int) (provisioning.Servers, error)
	GetByID(ctx context.Context, id int) (provisioning.Server, error)
}

type ServerClient interface {
	ImageServerClient
	InstanceServerClient
	NetworkServerClient
	NetworkACLServerClient
	NetworkForwardServerClient
	NetworkIntegrationServerClient
	NetworkLoadBalancerServerClient
	NetworkPeerServerClient
	NetworkZoneServerClient
	ProfileServerClient
	ProjectServerClient
	StorageBucketServerClient
	StoragePoolServerClient
	StorageVolumeServerClient
}
