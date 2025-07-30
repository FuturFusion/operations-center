package provisioning

import (
	"crypto/x509"
	"encoding/json"
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Server struct {
	ID                 int64
	Cluster            *string `db:"leftjoin=clusters.name"`
	Name               string  `db:"primary=yes"`
	Type               api.ServerType
	ConnectionURL      string
	Certificate        string
	ClusterCertificate *string `db:"omit=create,update&leftjoin=clusters.certificate"`
	HardwareData       api.HardwareData
	OSData             api.OSData
	VersionData        json.RawMessage `db:"ignore"` // FIXME: it is not yet clear, how the structure of the version information will actually look like.
	Status             api.ServerStatus
	LastUpdated        time.Time `db:"update_timestamp"`
	LastSeen           time.Time
}

func (s Server) Validate() error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid server, name can not be empty")
	}

	if s.Name == ":self" {
		return domain.NewValidationErrf(`Invalid server, ":self" is reserved for internal use and not allowed as server name`)
	}

	if s.ConnectionURL == "" {
		return domain.NewValidationErrf("Invalid server, connection URL can not be empty")
	}

	_, err := url.Parse(s.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid server, connection URL is not valid: %v", err)
	}

	if s.Certificate == "" {
		return domain.NewValidationErrf("Invalid server, certificate can not be empty")
	}

	var serverType api.ServerType
	err = serverType.UnmarshalText([]byte(s.Type))
	if s.Type == "" || err != nil {
		return domain.NewValidationErrf("Invalid server, validation of type failed: %v", err)
	}

	var serverStatus api.ServerStatus
	err = serverStatus.UnmarshalText([]byte(s.Status))
	if s.Status == "" || err != nil {
		return domain.NewValidationErrf("Invalid server, validation of status failed: %v", err)
	}

	return nil
}

type Servers []Server

type ServerFilter struct {
	Name        *string
	Cluster     *string
	Status      *api.ServerStatus
	Certificate *string
	Expression  *string `db:"ignore"`
}

func (f ServerFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Cluster != nil {
		query.Add("cluster", *f.Cluster)
	}

	if f.Status != nil {
		query.Add("status", string(*f.Status))
	}

	if f.Certificate != nil {
		query.Add("certificate", *f.Certificate)
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f ServerFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}

type ServerSelfUpdate struct {
	ConnectionURL             string
	AuthenticationCertificate *x509.Certificate
}

type ServerSystemNetwork = api.ServerSystemNetwork
