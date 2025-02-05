package provisioning

import "github.com/FuturFusion/operations-center/internal/domain"

type Cluster struct {
	ID              int
	Name            string
	ServerHostnames []string
}

func (c Cluster) Validate() error {
	if c.Name == "" {
		return domain.NewValidationErrf("Invalid cluster, name can not be empty")
	}

	if len(c.ServerHostnames) == 0 {
		return domain.NewValidationErrf("Invalid cluster, server hostname list can not be empty")
	}

	return nil
}

type Clusters []Cluster
