package entities

import (
	"encoding/json"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/shared/api"
)

// Code generation directives.
//
//generate-database:mapper target servers.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e server objects
//generate-database:mapper stmt -e server objects-by-Name
//generate-database:mapper stmt -e server objects-by-Cluster
//generate-database:mapper stmt -e server id
//generate-database:mapper stmt -e server create
//generate-database:mapper stmt -e server update
//generate-database:mapper stmt -e server rename
//generate-database:mapper stmt -e server delete-by-Name
//
//generate-database:mapper method -e server ID
//generate-database:mapper method -e server Exists
//generate-database:mapper method -e server GetOne
//generate-database:mapper method -e server GetMany
//generate-database:mapper method -e server Create
//generate-database:mapper method -e server Update
//generate-database:mapper method -e server Rename
//generate-database:mapper method -e server DeleteOne-by-Name

type Server struct {
	ID            int
	Cluster       string `db:"join=clusters.name"`
	Name          string `db:"primary=yes"`
	Type          api.ServerType
	ConnectionURL string
	HardwareData  MarshalableResource `db:"marshal=yes"`
	VersionData   json.RawMessage
	LastUpdated   time.Time
}

type ServerFilter struct {
	Name    *string
	Cluster *string
}

type MarshalableResource incusapi.Resources

// MarshalDB implements the Marshaler interface.
func (m MarshalableResource) MarshalDB() (string, error) {
	body, err := json.Marshal(m)
	return string(body), err
}

// UnmarshalDB implements the Unmarshaler interface.
func (m *MarshalableResource) UnmarshalDB(text string) error {
	return json.Unmarshal([]byte(text), m)
}
