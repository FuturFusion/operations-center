package entities

import (
	"strings"
	"time"
)

// Code generation directives.
//
//generate-database:mapper target clusters.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e cluster objects
//generate-database:mapper stmt -e cluster objects-by-Name
//generate-database:mapper stmt -e cluster id
//generate-database:mapper stmt -e cluster create
//generate-database:mapper stmt -e cluster update
//generate-database:mapper stmt -e cluster rename
//generate-database:mapper stmt -e cluster delete-by-Name
//
//generate-database:mapper method -e cluster ID
//generate-database:mapper method -e cluster Exists
//generate-database:mapper method -e cluster GetOne
//generate-database:mapper method -e cluster GetMany
//generate-database:mapper method -e cluster Create
//generate-database:mapper method -e cluster Update
//generate-database:mapper method -e cluster Rename
//generate-database:mapper method -e cluster DeleteOne-by-Name

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

// MarshalDB implements the Marshaler interface.
func (s StringSlice) MarshalDB() (string, error) {
	return strings.Join(s, ","), nil
}

// UnmarshalDB implements the Unmarshaler interface.
func (s *StringSlice) UnmarshalDB(text string) error {
	*s = StringSlice(strings.Split(text, ","))

	return nil
}
