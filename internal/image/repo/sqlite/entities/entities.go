package entities

//go:generate go run github.com/lxc/incus/v7/cmd/generate-database db mapper generate -b mapper_boilerplate.go -p "github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities" -p "github.com/FuturFusion/operations-center/internal/image"
//go:generate go run github.com/FuturFusion/operations-center/cmd/generate-format *.mapper.go
