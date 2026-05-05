package provisioning

import (
	"context"
	"crypto/x509"
	"io"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incus "github.com/lxc/incus/v6/client"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type ClusterService interface {
	Create(ctx context.Context, cluster Cluster) (Cluster, error)
	AddServers(ctx context.Context, name string, serverNames []string, skipPostJoinOperations bool) error
	RemoveServer(ctx context.Context, name string, removedServerNames []string) error
	GetAll(ctx context.Context) (Clusters, error)
	GetAllWithFilter(ctx context.Context, filter ClusterFilter) (Clusters, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetAllNamesWithFilter(ctx context.Context, filter ClusterFilter) ([]string, error)
	GetByName(ctx context.Context, name string) (*Cluster, error)
	Update(ctx context.Context, cluster Cluster, updateServers bool) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string, force bool) error
	DeleteAndFactoryResetByName(ctx context.Context, name string, tokenID *uuid.UUID, tokenSeedName *string) error
	ResyncInventory(ctx context.Context) error
	ResyncInventoryByName(ctx context.Context, name string) error
	StartLifecycleEventsMonitor(ctx context.Context) error
	UpdateCertificate(ctx context.Context, name string, certificatePEM string, keyPEM string) error
	GetEndpoint(ctx context.Context, name string) (Endpoint, error)
	IsInstanceLifecycleOperationPermitted(ctx context.Context, name string) bool
	LaunchClusterUpdate(ctx context.Context, name string, reboot bool) error
	ClusterUpdateControlLoop(ctx context.Context, clusterNameFilter *string) error
	AbortClusterUpdate(ctx context.Context, name string) error

	GetClusterArtifactAll(ctx context.Context, clusterName string) (ClusterArtifacts, error)
	GetClusterArtifactAllNames(ctx context.Context, clusterName string) ([]string, error)
	GetClusterArtifactByName(ctx context.Context, clusterName string, artifactName string) (*ClusterArtifact, error)
	GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error)
	GetClusterArtifactFileByName(ctx context.Context, clusterName string, artifactName string, filename string) (*ClusterArtifactFile, error)
	SetInventorySyncers(inventorySyncers map[domain.ResourceType]InventorySyncer)

	AddServerSystemNetworkVLANTags(ctx context.Context, clusterName string, interfaceName string, vlanTags []int) error
	RemoveServerSystemNetworkVLANTags(ctx context.Context, clusterName, interfaceName string, vlanTags []int) error
	UpdateSystemLogging(ctx context.Context, clusterName string, loggingConfig ServerSystemLogging) error
	UpdateSystemKernel(ctx context.Context, clusterName string, kerneConfig ServerSystemKernel) error
	AddApplication(ctx context.Context, clusterName string, applicationName string) error
	AddStorageTargetISCSI(ctx context.Context, clusterName string, target incusosapi.ServiceISCSITarget) error
	RemoveStorageTargetISCSI(ctx context.Context, clusterName string, target incusosapi.ServiceISCSITarget) error
	AddStorageTargetMultipath(ctx context.Context, clusterName string, target string) error
	RemoveStorageTargetMultipath(ctx context.Context, clusterName string, target string) error
	AddStorageTargetNVME(ctx context.Context, clusterName string, target incusosapi.ServiceNVMETarget) error
	RemoveStorageTargetNVME(ctx context.Context, clusterName string, target incusosapi.ServiceNVMETarget) error
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
	CreateClusterArtifactFromPath(ctx context.Context, artifact ClusterArtifact, path string, ignoredFiles []string) (int64, error)
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
	GetOSServiceLVM(ctx context.Context, server Server) (incusosapi.ServiceLVM, error)
	GetOSServiceISCSI(ctx context.Context, server Server) (incusosapi.ServiceISCSI, error)
	GetOSServiceMultipath(ctx context.Context, server Server) (incusosapi.ServiceMultipath, error)
	GetOSServiceNVME(ctx context.Context, server Server) (incusosapi.ServiceNVME, error)
	UpdateOSService(ctx context.Context, server Server, name string, config any) error
	UpdateNetworkConfig(ctx context.Context, server Server) error
	SetServerConfig(ctx context.Context, endpoint Endpoint, config map[string]string) error
	EnableCluster(ctx context.Context, server Server) (clusterCertificate string, _ error)
	GetClusterNodeNames(ctx context.Context, endpoint Endpoint) (nodeNames []string, _ error)
	GetClusterJoinToken(ctx context.Context, endpoint Endpoint, memberName string) (joinToken string, _ error)
	JoinCluster(ctx context.Context, server Server, joinToken string, serverAddressOfClusterRole string, endpoint Endpoint, config []api.ClusterMemberConfigKey) error
	GetOSData(ctx context.Context, endpoint Endpoint) (api.OSData, error)
	UpdateClusterCertificate(ctx context.Context, endpoint Endpoint, certificatePEM string, keyPEM string) error
	SystemFactoryReset(ctx context.Context, endpoint Endpoint, allowTPMResetFailure bool, seeds TokenImageSeedConfigs, providerConfig api.TokenProviderConfig) error
	SubscribeLifecycleEvents(ctx context.Context, endpoint Endpoint) (chan domain.LifecycleEvent, chan error, error)
	UpdateUpdateConfig(ctx context.Context, server Server, updateConfig ServerSystemUpdate) error
	GetNetworkConfig(ctx context.Context, server Server) (ServerSystemNetwork, error)
	GetStorageConfig(ctx context.Context, server Server) (ServerSystemStorage, error)

	GetRemoteCertificate(ctx context.Context, endpoint Endpoint) (*x509.Certificate, error)

	IncusClient(ctx context.Context, endpoint Endpoint) (InstanceServer, error)
}

type InstanceServer = incus.InstanceServer

type ClusterProvisioningPort interface {
	Init(ctx context.Context, clusterName string, config ClusterProvisioningConfig) (temporaryPath string, cleanup func() error, _ error)
	SeedCertificate(ctx context.Context, clusterName string, certificate string) error
	Apply(ctx context.Context, cluster Cluster) error
}
