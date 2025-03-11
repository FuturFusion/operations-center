package inventory

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type ProvisioningClusterService interface {
	GetAll(ctx context.Context) (provisioning.Clusters, error)
	GetByName(ctx context.Context, name string) (provisioning.Cluster, error)
}

type ProvisioningServerService interface {
	GetAllByClusterName(ctx context.Context, name string) (provisioning.Servers, error)
	GetByName(ctx context.Context, name string) (provisioning.Server, error)
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
