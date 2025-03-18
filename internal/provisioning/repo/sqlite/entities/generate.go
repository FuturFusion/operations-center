package entities

//go:generate go run github.com/lxc/incus/v6/cmd/generate-database db mapper generate -b mapper_boilerplate.go
// FIXME: Remove when https://github.com/lxc/incus/pull/1725 is merged.
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .
