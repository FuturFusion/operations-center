package provisioning

import "github.com/FuturFusion/operations-center/internal/domain"

type Server struct {
	ID        int
	ClusterID int
	Hostname  string
}

func (s Server) Validate() error {
	if s.Hostname == "" {
		return domain.NewValidationErrf("Invalid server, hostname can not be empty")
	}

	if s.ClusterID < 1 {
		return domain.NewValidationErrf("Invalid server, cluster id can not be less than 1")
	}

	return nil
}

type Servers []Server
