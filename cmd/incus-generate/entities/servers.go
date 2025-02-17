package entities

import (
	"time"

	"github.com/FuturFusion/operations-center/shared/api"
)

// Code generation directives.
//
//go:generate -command mapper go run github.com/FuturFusion/operations-center/cmd/generate-database db mapper -t servers.mapper.go
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
//
//go:generate go run github.com/piranha/goreplace github.com/lxc/incus/v6/internal/server/db/query --replace github.com/FuturFusion/operations-center/cmd/incus-generate/query --only servers.mapper.go --no-colors
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .

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
