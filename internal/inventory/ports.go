package inventory

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out mock/cluster_service_mock_gen.go -rm . ClusterService

type ClusterService interface {
	GetAll(ctx context.Context) (provisioning.Clusters, error)
	GetByID(ctx context.Context, id int) (provisioning.Cluster, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out mock/server_service_mock_gen.go -rm . ServerService

type ServerService interface {
	GetAllByClusterID(ctx context.Context, clusterID int) (provisioning.Servers, error)
	GetByID(ctx context.Context, id int) (provisioning.Server, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out mock/server_client_mock_gen.go -rm . ServerClient

type ServerClient interface {
	ImageServerClient
	InstanceServerClient
	NetworkServerClient
	NetworkACLServerClient
	NetworkIntegrationServerClient
	NetworkZoneServerClient
	ProfileServerClient
	ProjectServerClient
}
