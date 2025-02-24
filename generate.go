package tools

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --object-name-property-name filename image
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --object-type InstanceFull instance
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory network
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --object-name-property-name project network_acl
// TODO: network_forward
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --omit-project network_integration
// TODO: network_loadbalancer
// TODO: network_peer
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory network_zone
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory profile
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --omit-project project
