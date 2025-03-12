package provisioning

import (
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
)

type Cluster struct {
	Name          string
	ConnectionURL string
	ServerNames   []string
	LastUpdated   time.Time
}

func (c Cluster) Validate() error {
	if c.Name == "" {
		return domain.NewValidationErrf("Invalid cluster, name can not be empty")
	}

	if len(c.ServerNames) == 0 {
		return domain.NewValidationErrf("Invalid cluster, list of server names can not be empty")
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
