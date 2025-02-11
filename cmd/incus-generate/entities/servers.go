package entities

import (
	"time"

	"github.com/FuturFusion/operations-center/shared/api"
)

// Code generation directives.
//
//go:generate -command mapper incus-generate db mapper -t servers.mapper.go
//go:generate mapper reset
//
//go:generate mapper stmt -e server objects
//go:generate mapper stmt -e server objects-by-Hostname
//go:generate mapper stmt -e server id
//go:generate mapper stmt -e server create
//go:generate mapper stmt -e server delete-by-Hostname
//
//go:generate mapper method -e server ID
//go:generate mapper method -e server Exists
//go:generate mapper method -e server GetOne
//go:generate mapper method -e server GetMany
//go:generate mapper method -e server Create
//go:generate mapper method -e server DeleteOne-by-Hostname

type Server struct {
	ID            int
	ClusterID     int    `db:"join=clusters.id"`
	Hostname      string `db:"primary=yes"`
	Type          api.ServerType
	ConnectionURL string
	LastUpdated   time.Time
}

type ServerFilter struct {
	Hostname *string
}
