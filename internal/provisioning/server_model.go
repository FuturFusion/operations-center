package provisioning

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

//
//generate-expr: Server

type Server struct {
	ID                   int64            `json:"-"`
	Cluster              *string          `json:"cluster"                db:"leftjoin=clusters.name"`
	Name                 string           `json:"name"                   db:"primary=yes"`
	Type                 api.ServerType   `json:"type"`
	ConnectionURL        string           `json:"connection_url"`
	PublicConnectionURL  string           `json:"public_connection_url"`
	Certificate          string           `json:"certificate"`
	Fingerprint          string           `json:"fingerprint"            db:"ignore"`
	ClusterCertificate   *string          `json:"cluster_certificate"    db:"omit=create,update&leftjoin=clusters.certificate"`
	ClusterConnectionURL *string          `json:"cluster_connection_url" db:"omit=create,update&leftjoin=clusters.connection_url"`
	HardwareData         api.HardwareData `json:"hardware_data"`
	OSData               api.OSData       `json:"os_data"`
	VersionData          json.RawMessage  `json:"version_data"           db:"ignore"` // FIXME: it is not yet clear, how the structure of the version information will actually look like.
	Status               api.ServerStatus `json:"status"`
	LastUpdated          time.Time        `json:"last_updated"           db:"update_timestamp"`
	LastSeen             time.Time        `json:"last_seen"`
}

func (s Server) GetConnectionURL() string {
	return s.ConnectionURL
}

func (s Server) GetCertificate() string {
	if s.Cluster != nil {
		if s.ClusterCertificate != nil {
			return *s.ClusterCertificate
		}

		return ""
	}

	return s.Certificate
}

func (s Server) GetServerName() (string, error) {
	targetURL := s.ConnectionURL
	if s.ClusterConnectionURL != nil {
		targetURL = *s.ClusterConnectionURL
	}

	connectionURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("Failed to get server name from connection URL %q: %w", targetURL, err)
	}

	return connectionURL.Hostname(), nil
}

func (s Server) Validate() error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid server, name can not be empty")
	}

	if s.Name == ":self" {
		return domain.NewValidationErrf(`Invalid server, ":self" is reserved for internal use and not allowed as server name`)
	}

	if s.Type != api.ServerTypeOperationsCenter && s.ConnectionURL == "" {
		return domain.NewValidationErrf("Invalid server, connection URL can not be empty for server type %s", s.Type)
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
	Type        *api.ServerType
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

	if f.Type != nil {
		query.Add("type", f.Type.String())
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

	// Self is set to true, if the self update API has been called through
	// unix socket. This is the case, when IncusOS is serving Operations Center
	// and triggers a self update on its self.
	Self bool
}

type ServerSystemNetwork = api.ServerSystemNetwork

type ServerSystemStorage = api.ServerSystemStorage

type ServerSystemProvider = api.ServerSystemProvider
