package entities

import (
	"strings"
	"time"
)

// Code generation directives.
//
//go:generate -command mapper go run github.com/lxc/incus/v6/cmd/generate-database db mapper -t clusters.mapper.go
//go:generate mapper reset
//
//go:generate mapper stmt -e cluster objects
//go:generate mapper stmt -e cluster objects-by-Name
//go:generate mapper stmt -e cluster id
//go:generate mapper stmt -e cluster create
//go:generate mapper stmt -e cluster delete-by-Name
//
//go:generate mapper method -e cluster ID
//go:generate mapper method -e cluster Exists
//go:generate mapper method -e cluster GetOne
//go:generate mapper method -e cluster GetMany
//go:generate mapper method -e cluster Create
//go:generate mapper method -e cluster DeleteOne-by-Name
//
//go:generate go run github.com/piranha/goreplace github.com/lxc/incus/v6/internal/server/db/query --replace github.com/FuturFusion/operations-center/cmd/incus-generate/query --only clusters.mapper.go --no-colors
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .

type Cluster struct {
	ID              int
	Name            string `db:"primary=yes"`
	ConnectionURL   string
	ServerHostnames StringSlice `db:"marshal=yes"`
	LastUpdated     time.Time
}

type ClusterFilter struct {
	Name *string
}

type StringSlice []string

// MarshalText implements the encoding.TextMarshaler interface.
func (s StringSlice) MarshalDB() (string, error) {
	return strings.Join(s, ","), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *StringSlice) UnmarshalDB(text string) error {
	*s = StringSlice(strings.Split(text, ","))

	return nil
}
