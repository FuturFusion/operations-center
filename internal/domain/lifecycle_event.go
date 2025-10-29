package domain

import "strings"

type LifecycleEvent struct {
	ResourceType LifecycleResourceType
	Action       LifecycleAction
	Source       LifecycleSource
}

type LifecycleResourceType string

// nolint: godot
const (
	// LifecycleResourceTypeCertificate              LifecycleResourceType = "certificate"
	// LifecycleResourceTypeClusterCertificate       LifecycleResourceType = "cluster-certificate"
	// LifecycleResourceTypeCluster                  LifecycleResourceType = "cluster"
	// LifecycleResourceTypeClusterGroup             LifecycleResourceType = "cluster-group"
	// LifecycleResourceTypeClusterMember            LifecycleResourceType = "cluster-member"
	// LifecycleResourceTypeClusterToken             LifecycleResourceType = "cluster-token"
	// LifecycleResourceTypeConfig                   LifecycleResourceType = "config"
	// LifecycleResourceTypeImageAlias  LifecycleResourceType = "image-alias"
	LifecycleResourceTypeImage LifecycleResourceType = "image"
	// LifecycleResourceTypeImageSecret LifecycleResourceType = "image-secret"
	// LifecycleResourceTypeInstanceBackup           LifecycleResourceType = "instance-backup"
	LifecycleResourceTypeInstance LifecycleResourceType = "instance"
	// LifecycleResourceTypeInstanceConsole          LifecycleResourceType = "instance-console"
	// LifecycleResourceTypeInstanceFile             LifecycleResourceType = "instance-file"
	// LifecycleResourceTypeInstanceLog              LifecycleResourceType = "instance-log"
	// LifecycleResourceTypeInstanceMetadata LifecycleResourceType = "instance-metadata"
	// LifecycleResourceTypeInstanceMetadataTemplate LifecycleResourceType = "instance-metadata-template"
	// LifecycleResourceTypeInstanceSnapshot         LifecycleResourceType = "instance-snapshot"
	LifecycleResourceTypeNetworkACL          LifecycleResourceType = "network-acl"
	LifecycleResourceTypeNetworkAddressSet   LifecycleResourceType = "network-address-set"
	LifecycleResourceTypeNetwork             LifecycleResourceType = "network"
	LifecycleResourceTypeNetworkForward      LifecycleResourceType = "network-forward"
	LifecycleResourceTypeNetworkIntegration  LifecycleResourceType = "network-integration"
	LifecycleResourceTypeNetworkLoadBalancer LifecycleResourceType = "network-load-balancer"
	LifecycleResourceTypeNetworkPeer         LifecycleResourceType = "network-peer"
	LifecycleResourceTypeNetworkZone         LifecycleResourceType = "network-zone"
	// LifecycleResourceTypeNetworkZoneRecord        LifecycleResourceType = "network-zone-record"
	// LifecycleResourceTypeOperation                LifecycleResourceType = "operation"
	LifecycleResourceTypeProfile LifecycleResourceType = "profile"
	LifecycleResourceTypeProject LifecycleResourceType = "project"
	// LifecycleResourceTypeStorageBucketBackup      LifecycleResourceType = "storage-bucket-backup"
	LifecycleResourceTypeStorageBucket LifecycleResourceType = "storage-bucket"
	// LifecycleResourceTypeStorageBucketKey         LifecycleResourceType = "storage-bucket-key"
	LifecycleResourceTypeStoragePool LifecycleResourceType = "storage-pool"
	// LifecycleResourceTypeStorageVolumeBackup      LifecycleResourceType = "storage-volume-backup"
	LifecycleResourceTypeStorageVolume LifecycleResourceType = "storage-volume"
	// LifecycleResourceTypeStorageVolumeSnapshot    LifecycleResourceType = "storage-volume-snapshot"
	// LifecycleResourceTypeWarning                  LifecycleResourceType = "warning"
)

var LifecycleResources = map[LifecycleResourceType]struct{}{
	// LifecycleResourceTypeCertificate: {},
	// LifecycleResourceTypeClusterCertificate: {},
	// LifecycleResourceTypeCluster: {},
	// LifecycleResourceTypeClusterGroup: {},
	// LifecycleResourceTypeClusterMember: {},
	// LifecycleResourceTypeClusterToken: {},
	// LifecycleResourceTypeConfig: {},
	// LifecycleResourceTypeImageAlias: {},
	LifecycleResourceTypeImage: {},
	// LifecycleResourceTypeImageSecret: {},
	// LifecycleResourceTypeInstanceBackup: {},
	LifecycleResourceTypeInstance: {},
	// LifecycleResourceTypeInstanceConsole: {},
	// LifecycleResourceTypeInstanceFile: {},
	// LifecycleResourceTypeInstanceLog: {},
	// LifecycleResourceTypeInstanceMetadata: {},
	// LifecycleResourceTypeInstanceMetadataTemplate: {},
	// LifecycleResourceTypeInstanceSnapshot: {},
	LifecycleResourceTypeNetworkACL:          {},
	LifecycleResourceTypeNetworkAddressSet:   {},
	LifecycleResourceTypeNetwork:             {},
	LifecycleResourceTypeNetworkForward:      {},
	LifecycleResourceTypeNetworkIntegration:  {},
	LifecycleResourceTypeNetworkLoadBalancer: {},
	LifecycleResourceTypeNetworkPeer:         {},
	LifecycleResourceTypeNetworkZone:         {},
	// LifecycleResourceTypeNetworkZoneRecord: {},
	// LifecycleResourceTypeOperation: {},
	LifecycleResourceTypeProfile: {},
	LifecycleResourceTypeProject: {},
	// LifecycleResourceTypeStorageBucketBackup: {},
	LifecycleResourceTypeStorageBucket: {},
	// LifecycleResourceTypeStorageBucketKey: {},
	LifecycleResourceTypeStoragePool: {},
	// LifecycleResourceTypeStorageVolumeBackup: {},
	LifecycleResourceTypeStorageVolume: {},
	// LifecycleResourceTypeStorageVolumeSnapshot: {},
	// LifecycleResourceTypeWarning: {},
}

type LifecycleAction string

const (
	LifecycleActionCreate LifecycleAction = "create" // = refresh?
	LifecycleActionUpdate LifecycleAction = "update" // = refresh?
	LifecycleActionDelete LifecycleAction = "delete"
	// TODO: rename? or is this delete + refresh?
)

type LifecycleSource struct {
	ProjectName string
	ParentType  string
	ParentName  string
	Name        string
	Type        string
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
