package entities

//go:generate go run github.com/lxc/incus/v7/cmd/generate-database db mapper generate -b mapper_boilerplate.go -p "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities" -p "github.com/FuturFusion/operations-center/internal/provisioning"
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w -local github.com/FuturFusion/operations-center .
