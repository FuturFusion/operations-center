package domain

import (
	"strings"

	incusapi "github.com/lxc/incus/v6/shared/api"
)

type LifecycleEvent struct {
	ResourceType ResourceType
	Operation    LifecycleOperation
	Source       LifecycleSource
}

type ResourceType string

const (
	ResourceTypeImage               ResourceType = "image"
	ResourceTypeInstance            ResourceType = "instance"
	ResourceTypeNetwork             ResourceType = "network"
	ResourceTypeNetworkACL          ResourceType = "network-acl"
	ResourceTypeNetworkAddressSet   ResourceType = "network-address-set"
	ResourceTypeNetworkForward      ResourceType = "network-forward"
	ResourceTypeNetworkIntegration  ResourceType = "network-integration"
	ResourceTypeNetworkLoadBalancer ResourceType = "network-load-balancer"
	ResourceTypeNetworkPeer         ResourceType = "network-peer"
	ResourceTypeNetworkZone         ResourceType = "network-zone"
	ResourceTypeProfile             ResourceType = "profile"
	ResourceTypeProject             ResourceType = "project"
	ResourceTypeServer              ResourceType = "server"
	ResourceTypeStorageBucket       ResourceType = "storage-bucket"
	ResourceTypeStoragePool         ResourceType = "storage-pool"
	ResourceTypeStorageVolume       ResourceType = "storage-volume"
)

type LifecycleOperation string

const (
	LifecycleOperationCreate   LifecycleOperation = "create"
	LifecycleOperationUpdate   LifecycleOperation = "update"
	LifecycleOperationDelete   LifecycleOperation = "delete"
	LifecycleOperationRename   LifecycleOperation = "rename"
	LifecycleOperationEvacuate LifecycleOperation = "evacuate"
	LifecycleOperationRestore  LifecycleOperation = "restore"
)

type LifecycleSource struct {
	ProjectName string
	ParentType  string
	ParentName  string
	Name        string
	Type        string
	OldName     string
}

func (l LifecycleSource) String() string {
	buf := strings.Builder{}

	buf.WriteString("/")

	if l.ParentName != "" {
		buf.WriteString(l.ParentType + "/" + l.ParentName + "/")
	}

	if l.Type != "" {
		buf.WriteString(l.Type + "/")
	}

	buf.WriteString(l.Name)

	if l.ProjectName != "" {
		buf.WriteString("?project=" + l.ProjectName)
	}

	return buf.String()
}

var MapLifecycleAction = map[string]LifecycleEvent{
	// incusapi.EventLifecycleCertificateCreated: {},
	// incusapi.EventLifecycleCertificateDeleted: {},
	// incusapi.EventLifecycleCertificateUpdated: {},
	// incusapi.EventLifecycleClusterCertificateUpdated: {},
	// incusapi.EventLifecycleClusterDisabled: {},
	// incusapi.EventLifecycleClusterEnabled: {},
	// incusapi.EventLifecycleClusterGroupCreated: {},
	// incusapi.EventLifecycleClusterGroupDeleted: {},
	// incusapi.EventLifecycleClusterGroupRenamed: {},
	// incusapi.EventLifecycleClusterGroupUpdated: {},
	// incusapi.EventLifecycleClusterMemberAdded: {},
	incusapi.EventLifecycleClusterMemberEvacuated: {
		ResourceType: ResourceTypeServer,
		Operation:    LifecycleOperationEvacuate,
	},
	// incusapi.EventLifecycleClusterMemberHealed: {},
	// incusapi.EventLifecycleClusterMemberRemoved: {},
	// incusapi.EventLifecycleClusterMemberRenamed: {},
	incusapi.EventLifecycleClusterMemberRestored: {
		ResourceType: ResourceTypeServer,
		Operation:    LifecycleOperationRestore,
	},
	// incusapi.EventLifecycleClusterMemberUpdated: {},
	// incusapi.EventLifecycleClusterTokenCreated: {},
	// incusapi.EventLifecycleConfigUpdated: {},
	incusapi.EventLifecycleImageAliasCreated: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleImageAliasDeleted: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleImageAliasRenamed: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleImageAliasUpdated: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleImageCreated: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleImageDeleted: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleImageRefreshed: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleImageRetrieved: {},
	// incusapi.EventLifecycleImageSecretCreated: {},
	incusapi.EventLifecycleImageUpdated: {
		ResourceType: ResourceTypeImage,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceBackupCreated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceBackupDeleted: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceBackupRenamed: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleInstanceBackupRetrieved: {},
	// incusapi.EventLifecycleInstanceConsole: {},
	// incusapi.EventLifecycleInstanceConsoleReset: {},
	// incusapi.EventLifecycleInstanceConsoleRetrieved: {},
	incusapi.EventLifecycleInstanceCreated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleInstanceDeleted: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationDelete,
	},
	// incusapi.EventLifecycleInstanceExec: {},
	// incusapi.EventLifecycleInstanceFileDeleted: {},
	// incusapi.EventLifecycleInstanceFilePushed: {},
	// incusapi.EventLifecycleInstanceFileRetrieved: {},
	// incusapi.EventLifecycleInstanceLogDeleted: {},
	// incusapi.EventLifecycleInstanceLogRetrieved: {},
	// incusapi.EventLifecycleInstanceMetadataRetrieved: {},
	// incusapi.EventLifecycleInstanceMetadataTemplateCreated: {},
	// incusapi.EventLifecycleInstanceMetadataTemplateDeleted: {},
	// incusapi.EventLifecycleInstanceMetadataTemplateRetrieved: {},
	incusapi.EventLifecycleInstanceMetadataUpdated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceMigrated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstancePaused: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceReady: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceRenamed: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleInstanceRestarted: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceRestored: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceResumed: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceShutdown: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceSnapshotCreated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceSnapshotDeleted: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceSnapshotRenamed: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceSnapshotUpdated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceStarted: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceStopped: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleInstanceUpdated: {
		ResourceType: ResourceTypeInstance,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkACLCreated: {
		ResourceType: ResourceTypeNetworkACL,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkACLDeleted: {
		ResourceType: ResourceTypeNetworkACL,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkACLRenamed: {
		ResourceType: ResourceTypeNetworkACL,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleNetworkACLUpdated: {
		ResourceType: ResourceTypeNetworkACL,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkAddressSetCreated: {
		ResourceType: ResourceTypeNetworkAddressSet,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkAddressSetDeleted: {
		ResourceType: ResourceTypeNetworkAddressSet,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkAddressSetRenamed: {
		ResourceType: ResourceTypeNetworkAddressSet,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleNetworkAddressSetUpdated: {
		ResourceType: ResourceTypeNetworkAddressSet,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkCreated: {
		ResourceType: ResourceTypeNetwork,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkDeleted: {
		ResourceType: ResourceTypeNetwork,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkForwardCreated: {
		ResourceType: ResourceTypeNetworkForward,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkForwardDeleted: {
		ResourceType: ResourceTypeNetworkForward,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkForwardUpdated: {
		ResourceType: ResourceTypeNetworkForward,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkIntegrationCreated: {
		ResourceType: ResourceTypeNetworkIntegration,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkIntegrationDeleted: {
		ResourceType: ResourceTypeNetworkIntegration,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkIntegrationRenamed: {
		ResourceType: ResourceTypeNetworkIntegration,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleNetworkIntegrationUpdated: {
		ResourceType: ResourceTypeNetworkIntegration,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkLoadBalancerCreated: {
		ResourceType: ResourceTypeNetworkLoadBalancer,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkLoadBalancerDeleted: {
		ResourceType: ResourceTypeNetworkLoadBalancer,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkLoadBalancerUpdated: {
		ResourceType: ResourceTypeNetworkLoadBalancer,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkPeerCreated: {
		ResourceType: ResourceTypeNetworkPeer,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkPeerDeleted: {
		ResourceType: ResourceTypeNetworkPeer,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkPeerUpdated: {
		ResourceType: ResourceTypeNetworkPeer,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkRenamed: {
		ResourceType: ResourceTypeNetwork,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleNetworkUpdated: {
		ResourceType: ResourceTypeNetwork,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkZoneCreated: {
		ResourceType: ResourceTypeNetworkZone,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleNetworkZoneDeleted: {
		ResourceType: ResourceTypeNetworkZone,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleNetworkZoneRecordCreated: {
		ResourceType: ResourceTypeNetworkZone,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkZoneRecordDeleted: {
		ResourceType: ResourceTypeNetworkZone,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkZoneRecordUpdated: {
		ResourceType: ResourceTypeNetworkZone,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleNetworkZoneUpdated: {
		ResourceType: ResourceTypeNetworkZone,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleOperationCancelled: {},
	incusapi.EventLifecycleProfileCreated: {
		ResourceType: ResourceTypeProfile,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleProfileDeleted: {
		ResourceType: ResourceTypeProfile,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleProfileRenamed: {
		ResourceType: ResourceTypeProfile,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleProfileUpdated: {
		ResourceType: ResourceTypeProfile,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleProjectCreated: {
		ResourceType: ResourceTypeProject,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleProjectDeleted: {
		ResourceType: ResourceTypeProject,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleProjectRenamed: {
		ResourceType: ResourceTypeProject,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleProjectUpdated: {
		ResourceType: ResourceTypeProject,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleStorageBucketBackupCreated: {},
	// incusapi.EventLifecycleStorageBucketBackupDeleted: {},
	// incusapi.EventLifecycleStorageBucketBackupRenamed: {},
	// incusapi.EventLifecycleStorageBucketBackupRetrieved: {},
	incusapi.EventLifecycleStorageBucketCreated: {
		ResourceType: ResourceTypeStorageBucket,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleStorageBucketDeleted: {
		ResourceType: ResourceTypeStorageBucket,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleStorageBucketKeyCreated: {
		ResourceType: ResourceTypeStorageBucket,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleStorageBucketKeyDeleted: {
		ResourceType: ResourceTypeStorageBucket,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleStorageBucketKeyUpdated: {
		ResourceType: ResourceTypeStorageBucket,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleStorageBucketUpdated: {
		ResourceType: ResourceTypeStorageBucket,
		Operation:    LifecycleOperationUpdate,
	},
	incusapi.EventLifecycleStoragePoolCreated: {
		ResourceType: ResourceTypeStoragePool,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleStoragePoolDeleted: {
		ResourceType: ResourceTypeStoragePool,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleStoragePoolUpdated: {
		ResourceType: ResourceTypeStoragePool,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleStorageVolumeBackupCreated: {},
	// incusapi.EventLifecycleStorageVolumeBackupDeleted: {},
	// incusapi.EventLifecycleStorageVolumeBackupRenamed: {},
	// incusapi.EventLifecycleStorageVolumeBackupRetrieved: {},
	incusapi.EventLifecycleStorageVolumeCreated: {
		ResourceType: ResourceTypeStorageVolume,
		Operation:    LifecycleOperationCreate,
	},
	incusapi.EventLifecycleStorageVolumeDeleted: {
		ResourceType: ResourceTypeStorageVolume,
		Operation:    LifecycleOperationDelete,
	},
	incusapi.EventLifecycleStorageVolumeRenamed: {
		ResourceType: ResourceTypeStorageVolume,
		Operation:    LifecycleOperationRename,
	},
	incusapi.EventLifecycleStorageVolumeRestored: {
		ResourceType: ResourceTypeStorageVolume,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleStorageVolumeSnapshotCreated: {},
	// incusapi.EventLifecycleStorageVolumeSnapshotDeleted: {},
	// incusapi.EventLifecycleStorageVolumeSnapshotRenamed: {},
	// incusapi.EventLifecycleStorageVolumeSnapshotUpdated: {},
	incusapi.EventLifecycleStorageVolumeUpdated: {
		ResourceType: ResourceTypeStorageVolume,
		Operation:    LifecycleOperationUpdate,
	},
	// incusapi.EventLifecycleWarningAcknowledged: {},
	// incusapi.EventLifecycleWarningDeleted: {},
	// incusapi.EventLifecycleWarningReset: {},
}
