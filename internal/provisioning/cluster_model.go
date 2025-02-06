package provisioning

import (
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
)

type Cluster struct {
	ID              int
	Name            string
	ConnectionURL   string
	ServerHostnames []string // FIXME: it is unclear, if this is needed. If we don't have this, how can we know the expected size of a cluster and prevent it from growing / shrinking?
	LastUpdated     time.Time
}

func (c Cluster) Validate() error {
	if c.Name == "" {
		return domain.NewValidationErrf("Invalid cluster, name can not be empty")
	}

	if len(c.ServerHostnames) == 0 {
		return domain.NewValidationErrf("Invalid cluster, server hostname list can not be empty")
	}

	if c.ConnectionURL == "" {
		return domain.NewValidationErrf("Invalid cluster, connection URL can not be empty")
	}

	_, err := url.Parse(c.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid cluster, connection URL is not valid: %v", err)
	}

	return nil
}

type Clusters []Cluster
