package provisioning

import (
	"encoding/json"
	"net/url"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Server struct {
	Cluster       string
	Name          string
	Type          api.ServerType
	ConnectionURL string
	HardwareData  incusapi.Resources
	VersionData   json.RawMessage // FIXME: it is not yet clear, how the structure of the version information will actually look like.
	LastUpdated   time.Time
}

func (s Server) Validate() error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid server, name can not be empty")
	}

	if s.Cluster == "" {
		return domain.NewValidationErrf("Invalid server, cluster id can not be less than 1")
	}

	if s.ConnectionURL == "" {
		return domain.NewValidationErrf("Invalid server, connection URL can not be empty")
	}

	_, err := url.Parse(s.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid server, connection URL is not valid: %v", err)
	}

	return nil
}

type Servers []Server
