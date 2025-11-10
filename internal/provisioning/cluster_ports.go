package provisioning

import (
	"context"
	"io"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type ClusterService interface {
	Create(ctx context.Context, cluster Cluster) (Cluster, error)
	GetAll(ctx context.Context) (Clusters, error)
	GetAllWithFilter(ctx context.Context, filter ClusterFilter) (Clusters, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetAllNamesWithFilter(ctx context.Context, filter ClusterFilter) ([]string, error)
	GetByName(ctx context.Context, name string) (*Cluster, error)
	Update(ctx context.Context, cluster Cluster) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string, deleteMode api.ClusterDeleteMode) error
	ResyncInventory(ctx context.Context) error
	ResyncInventoryByName(ctx context.Context, name string) error
	StartLifecycleEventsMonitor(ctx context.Context) error
	UpdateCertificate(ctx context.Context, name string, certificatePEM string, keyPEM string) error
	GetEndpoint(ctx context.Context, name string) (Endpoint, error)
	GetProvisionerConfigurationArchive(ctx context.Context, name string) (_ io.ReadCloser, size int, _ error)
	GetClusterArtifactAll(ctx context.Context, clusterName string) (ClusterArtifacts, error)
	GetClusterArtifactAllNames(ctx context.Context, clusterName string) ([]string, error)
	GetClusterArtifactByName(ctx context.Context, clusterName string, artifactName string) (*ClusterArtifact, error)
	GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error)
	GetClusterArtifactFileByName(ctx context.Context, clusterName string, artifactName string, filename string) (*ClusterArtifactFile, error)
	SetInventorySyncers(inventorySyncers map[domain.ResourceType]InventorySyncer)
}

type ClusterRepo interface {
	Create(ctx context.Context, cluster Cluster) (int64, error)
	GetAll(ctx context.Context) (Clusters, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Cluster, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
	Update(ctx context.Context, cluster Cluster) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
}

type ClusterArtifactRepo interface {
	CreateClusterArtifactFromPath(ctx context.Context, artifact ClusterArtifact, path string) (int64, error)
	GetClusterArtifactAll(ctx context.Context, clusterName string) (ClusterArtifacts, error)
	GetClusterArtifactAllNames(ctx context.Context, clusterName string) ([]string, error)
	GetClusterArtifactByName(ctx context.Context, clusterName string, artifactName string) (*ClusterArtifact, error)
	GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error)
}

type InventorySyncer interface {
	SyncCluster(ctx context.Context, clusterName string) error
	ResyncByName(ctx context.Context, clusterName string, sourceDetails domain.LifecycleEvent) error
}

type ClusterClientPort interface {
	Ping(ctx context.Context, endpoint Endpoint) error
	EnableOSService(ctx context.Context, server Server, name string, config map[string]any) error
	SetServerConfig(ctx context.Context, endpoint Endpoint, config map[string]string) error
	EnableCluster(ctx context.Context, server Server) (clusterCertificate string, _ error)
	GetClusterNodeNames(ctx context.Context, endpoint Endpoint) (nodeNames []string, _ error)
	GetClusterJoinToken(ctx context.Context, endpoint Endpoint, memberName string) (joinToken string, _ error)
	JoinCluster(ctx context.Context, server Server, joinToken string, endpoint Endpoint) error
	GetOSData(ctx context.Context, endpoint Endpoint) (api.OSData, error)
	UpdateClusterCertificate(ctx context.Context, endpoint Endpoint, certificatePEM string, keyPEM string) error
	FactoryReset(ctx context.Context, endpoint Endpoint) error
	SubscribeLifecycleEvents(ctx context.Context, endpoint Endpoint) (chan domain.LifecycleEvent, chan error, error)
}

type ClusterProvisioningPort interface {
	Init(ctx context.Context, clusterName string, config ClusterProvisioningConfig) error
	Apply(ctx context.Context, cluster Cluster) error
	GetArchive(ctx context.Context, name string) (_ io.ReadCloser, size int, _ error)
}
