package tools

//go:generate -command generate-inventory go run github.com/FuturFusion/operations-center/cmd/generate-inventory
//go:generate generate-inventory --object-name-property-name filename image
//go:generate generate-inventory --object-type InstanceFull instance
//go:generate generate-inventory network
//go:generate generate-inventory --uses-embedded-post-type network_acl
//go:generate generate-inventory --object-name-property-name listen_address --omit-project --parent network network_forward
//go:generate generate-inventory --omit-project network_integration
//go:generate generate-inventory --object-name-property-name listen_address --omit-project --parent network network_load_balancer
//go:generate generate-inventory --omit-project --parent network network_peer
//go:generate generate-inventory network_zone
//go:generate generate-inventory profile
//go:generate generate-inventory --omit-project project
//go:generate generate-inventory --parent storage_pool storage_bucket
//go:generate generate-inventory --omit-project storage_pool
//go:generate generate-inventory --parent storage_pool storage_volume
