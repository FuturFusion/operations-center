package provisioning

import (
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Cluster struct {
	ID            int64
	Name          string `db:"primary=yes"`
	ConnectionURL string
	Certificate   string
	Status        api.ClusterStatus
	ServerNames   []string `db:"ignore"`
	LastUpdated   time.Time
}

func (c Cluster) Validate() error {
	if c.Name == "" {
		return domain.NewValidationErrf("Invalid cluster, name can not be empty")
	}

	if len(c.ServerNames) == 0 {
		return domain.NewValidationErrf("Invalid cluster, list of server names can not be empty")
	}

	_, err := url.Parse(c.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid cluster, connection URL is not valid: %v", err)
	}

	return nil
}

type Clusters []Cluster

type ClusterFilter struct {
	Name       *string
	Expression *string `db:"ignore"`
}

func (f ClusterFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f ClusterFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}
