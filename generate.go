package tools

//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --object-name-property-name filename image
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --object-type InstanceFull instance
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory network
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-inventory --object-name-property-name project network_acl
