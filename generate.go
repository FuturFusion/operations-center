package tools

//go:generate -command generate-inventory go run github.com/FuturFusion/operations-center/cmd/generate-inventory
//go:generate generate-inventory --object-name-property-name filename --incus-get-all-method GetImagesAllProjects --incus-get-method GetImage image
//go:generate generate-inventory --object-type InstanceFull --server-resource --incus-get-all-method GetInstancesFullAllProjects --incus-get-method GetInstanceFull instance
//go:generate generate-inventory --incus-get-all-method GetNetworksAllProjects --incus-get-method GetNetwork network
//go:generate generate-inventory --uses-embedded-post-type --incus-get-all-method GetNetworkACLsAllProjects --incus-get-method GetNetworkACL network_acl
//go:generate generate-inventory --object-name-property-name listen_address --omit-project --parent network --incus-get-all-method GetNetworkForwards --incus-get-method GetNetworkForward network_forward
//go:generate generate-inventory --omit-project --incus-get-all-method GetNetworkIntegrations --incus-get-method GetNetworkIntegration network_integration
//go:generate generate-inventory --object-name-property-name listen_address --omit-project --parent network --incus-get-all-method GetNetworkLoadBalancers --incus-get-method GetNetworkLoadBalancer network_load_balancer
//go:generate generate-inventory --omit-project --parent network --incus-get-all-method GetNetworkPeers --incus-get-method GetNetworkPeer network_peer
//go:generate generate-inventory --incus-get-all-method GetNetworkZonesAllProjects --incus-get-method GetNetworkZone network_zone
//go:generate generate-inventory --incus-get-all-method GetProfilesAllProjects --incus-get-method GetProfile profile
//go:generate generate-inventory --omit-project --incus-get-all-method GetProjects --incus-get-method GetProject project
//go:generate generate-inventory --server-resource --parent storage_pool --incus-get-all-method GetStoragePoolBucketsAllProjects --incus-get-method GetStoragePoolBucket storage_bucket
//go:generate generate-inventory --omit-project --incus-get-all-method GetStoragePools --incus-get-method GetStoragePool storage_pool
//go:generate generate-inventory --server-resource --parent storage_pool --incus-get-all-method GetStoragePoolVolumesAllProjects --incus-get-method GetStoragePoolVolume storage_volume

//go:generate go run github.com/vektra/mockery/v3 --log-level WARN
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-slog.yaml
//go:generate go run github.com/vektra/mockery/v3 --log-level WARN --config .mockery-prometheus.yaml
