package tools

//go:generate -command generate-inventory go run github.com/FuturFusion/operations-center/cmd/generate-inventory
//go:generate generate-inventory --object-name-property-name filename --incus-get-method GetImagesAllProjects image
//go:generate generate-inventory --object-type InstanceFull --incus-get-method GetInstancesFullAllProjects instance
//go:generate generate-inventory --incus-get-method GetNetworksAllProjects network
//go:generate generate-inventory --uses-embedded-post-type --incus-get-method GetNetworkACLsAllProjects network_acl
//go:generate generate-inventory --object-name-property-name listen_address --omit-project --parent network --incus-get-method GetNetworkForwards network_forward
//go:generate generate-inventory --omit-project --incus-get-method GetNetworkIntegrations network_integration
//go:generate generate-inventory --object-name-property-name listen_address --omit-project --parent network --incus-get-method GetNetworkLoadBalancers network_load_balancer
//go:generate generate-inventory --omit-project --parent network --incus-get-method GetNetworkPeers network_peer
//go:generate generate-inventory --incus-get-method GetNetworkZonesAllProjects network_zone
//go:generate generate-inventory --incus-get-method GetProfilesAllProjects profile
//go:generate generate-inventory --omit-project --incus-get-method GetProjects project
//go:generate generate-inventory --parent storage_pool --incus-get-method GetStoragePoolBucketsAllProjects storage_bucket
//go:generate generate-inventory --omit-project --incus-get-method GetStoragePools storage_pool
//go:generate generate-inventory --parent storage_pool --incus-get-method GetStoragePoolVolumesAllProjects storage_volume
