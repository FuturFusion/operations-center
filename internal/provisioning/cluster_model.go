package provisioning

import (
	"encoding/json"
	"fmt"
	"iter"
	"net/url"
	"strings"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Cluster struct {
	ID                    int64
	Name                  string `db:"primary=yes"`
	ConnectionURL         string
	Certificate           string
	Status                api.ClusterStatus
	ServerNames           []string       `db:"ignore"`
	ServerType            api.ServerType `db:"ignore"`
	ServicesConfig        map[string]any `db:"ignore"`
	ApplicationSeedConfig map[string]any `db:"ignore"`
	LastUpdated           time.Time      `db:"update_timestamp"`
}

const nameProhibitedCharacters = `\/:*?"<>|`

func (c Cluster) Validate() error {
	if c.Name == "" {
		return domain.NewValidationErrf("Invalid cluster, name can not be empty")
	}

	if strings.ContainsAny(c.Name, nameProhibitedCharacters) {
		return domain.NewValidationErrf("Invalid cluster, name can not contain any of %q", nameProhibitedCharacters)
	}

	_, err := url.Parse(c.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid cluster, connection URL is not valid: %v", err)
	}

	return nil
}

func (c Cluster) ValidateCreate() error {
	err := c.Validate()
	if err != nil {
		return err
	}

	if len(c.ServerNames) == 0 {
		return domain.NewValidationErrf("Invalid cluster, list of server names can not be empty")
	}

	if c.ServerType == api.ServerTypeUnknown || c.ServerType == "" {
		return domain.NewValidationErrf("Invalid cluster definition, server type can not be %q", c.ServerType)
	}

	body, err := json.Marshal(c.ApplicationSeedConfig)
	if err != nil {
		return domain.NewValidationErrf("Invalid cluster definition, failed to marshal application seed config: %v", err)
	}

	preseed := incusapi.InitLocalPreseed{
		ServerPut: incusapi.ServerPut{
			Config: map[string]string{},
		},
	}

	err = json.Unmarshal(body, &preseed)
	if err != nil {
		return domain.NewValidationErrf("Invalid cluster definition, failed to unmarshal application seed config as Incus local init preseed: %v", err)
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

type ClusterEndpoint []Server

func (c ClusterEndpoint) GetConnectionURL() string {
	if len(c) == 0 {
		return ""
	}

	return c[0].GetConnectionURL()
}

func (c ClusterEndpoint) GetCertificate() string {
	if len(c) == 0 {
		return ""
	}

	return c[0].GetCertificate()
}

func (c ClusterEndpoint) GetServerName() (string, error) {
	if len(c) == 0 {
		return "", fmt.Errorf("Failed to get server name, cluster does not have any servers")
	}

	return c[0].GetServerName()
}

func (c ClusterEndpoint) GetEndpoints() iter.Seq[Endpoint] {
	return func(yield func(Endpoint) bool) {
		for _, server := range c {
			cont := yield(server)
			if !cont {
				return
			}
		}
	}
}

type ClusterProvisioningConfig struct {
	ClusterEndpoint ClusterEndpoint
	Servers         []Server

	ApplicationSeedConfig map[string]any
}

type ClusterUpdateMessage struct {
	Operation ClusterUpdateOperation
	Name      string
	OldName   string
}

type ClusterUpdateOperation string

const (
	ClusterUpdateOperationCreate ClusterUpdateOperation = "create"
	ClusterUpdateOperationDelete ClusterUpdateOperation = "delete"
	ClusterUpdateOperationRename ClusterUpdateOperation = "rename"
)
