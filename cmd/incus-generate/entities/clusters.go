package entities

import (
	"strings"
	"time"
)

// Code generation directives.
//
//go:generate -command mapper go run github.com/FuturFusion/operations-center/cmd/generate-database db mapper -t clusters.mapper.go
//go:generate mapper reset --no-incus-api
//
//go:generate mapper stmt -e cluster objects
//go:generate mapper stmt -e cluster objects-by-Name
//go:generate mapper stmt -e cluster id
//go:generate mapper stmt -e cluster create
//go:generate mapper stmt -e cluster delete-by-Name
//
//go:generate mapper method -e cluster ID --map-error
//go:generate mapper method -e cluster Exists --map-error
//go:generate mapper method -e cluster GetOne --map-error
//go:generate mapper method -e cluster GetMany --map-error
//go:generate mapper method -e cluster Create --map-error
//go:generate mapper method -e cluster DeleteOne-by-Name --map-error
//
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
