package inventory

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out ./mock/cluster_service_mock_gen.go -rm . ClusterService

type ClusterService interface {
	GetAll(ctx context.Context) (provisioning.Clusters, error)
	GetByID(ctx context.Context, id int) (provisioning.Cluster, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out ./mock/server_service_mock_gen.go -rm . ServerService

type ServerService interface {
	GetAllByClusterID(ctx context.Context, clusterID int) (provisioning.Servers, error)
	GetByID(ctx context.Context, id int) (provisioning.Server, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out ./server/mock/server_client_mock_gen.go -rm . ServerClient
//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ServerClient -t ../logger/slog.gotmpl -o ./server/middleware/server_client_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i ServerClient -t prometheus -o ./server/middleware/server_client_prometheus_gen.go

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
