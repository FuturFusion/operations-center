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
	ID            int64
	Cluster       *string `db:"leftjoin=clusters.name"`
	Name          string  `db:"primary=yes"`
	Type          api.ServerType
	ConnectionURL string
	Certificate   string
	HardwareData  incusapi.Resources `db:"ignore"`
	VersionData   json.RawMessage    `db:"ignore"` // FIXME: it is not yet clear, how the structure of the version information will actually look like.
	Status        api.ServerStatus
	LastUpdated   time.Time
}

func (s Server) Validate() error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid server, name can not be empty")
	}

	if s.ConnectionURL == "" {
		return domain.NewValidationErrf("Invalid server, connection URL can not be empty")
	}

	if s.Certificate == "" {
		return domain.NewValidationErrf("Invalid server, certificate can not be empty")
	}

	switch s.Status {
	case api.ServerStatusPending, api.ServerStatusReady:
	default:
		return domain.NewValidationErrf("Invalid server, status %q not supported", s.Status)
	}

	_, err := url.Parse(s.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid server, connection URL is not valid: %v", err)
	}

	return nil
}

type Servers []Server

type ServerFilter struct {
	Name       *string
	Cluster    *string
	Expression *string `db:"ignore"`
}

func (f ServerFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Cluster != nil {
		query.Add("cluster", *f.Cluster)
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f ServerFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}
