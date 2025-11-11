package entities

//go:generate go run github.com/lxc/incus/v6/cmd/generate-database db mapper generate -b mapper_boilerplate.go -p "github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities" -p "github.com/FuturFusion/operations-center/internal/provisioning"
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .
