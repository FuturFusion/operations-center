package provisioning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/url"
	"strings"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

//
//generate-expr: Cluster

type Cluster struct {
	ID                    int64             `json:"-"`
	Name                  string            `json:"name"                    db:"primary=yes"`
	ConnectionURL         string            `json:"connection_url"`
	Certificate           *string           `json:"certificate"`
	Fingerprint           string            `json:"fingerprint"             db:"ignore"`
	Status                api.ClusterStatus `json:"status"`
	ServerNames           []string          `json:"server_names"            db:"ignore"`
	ServerType            api.ServerType    `json:"server_type"             db:"ignore"`
	ServicesConfig        map[string]any    `json:"services_config"         db:"ignore"`
	ApplicationSeedConfig map[string]any    `json:"application_seed_config" db:"ignore"`
	Channel               string            `json:"channel"                 db:"join=channels.name"`
	LastUpdated           time.Time         `json:"last_updated"            db:"update_timestamp"`
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

	if c.Channel == "" {
		return domain.NewValidationErrf("Invalid cluster, channel can not be empty")
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

type ClusterArtifact struct {
	ID          int64
	Cluster     string `db:"primary=yes&join=clusters.name"`
	Name        string `db:"primary=yes"`
	Description string
	Properties  api.ConfigMap
	Files       ClusterArtifactFiles
	LastUpdated time.Time `db:"update_timestamp"`
}

type ClusterArtifactFile struct {
	Name     string
	MimeType string
	Size     int64
	Open     func() (io.ReadCloser, error) `db:"ignore"`
}

type ClusterArtifactFiles []ClusterArtifactFile

// Value implements the sql driver.Valuer interface.
func (c ClusterArtifactFiles) Value() (driver.Value, error) {
	files := make([]map[string]any, 0, len(c))

	for _, file := range c {
		files = append(files, map[string]any{
			"Name":     file.Name,
			"MimeType": file.MimeType,
			"Size":     file.Size,
		})
	}

	return json.Marshal(files)
}

// Scan implements the sql.Scanner interface.
func (c *ClusterArtifactFiles) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid cluster artifact files")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*c = ClusterArtifactFiles{}
			return nil
		}

		return json.Unmarshal([]byte(v), c)

	case []byte:
		if len(v) == 0 {
			*c = ClusterArtifactFiles{}
			return nil
		}

		return json.Unmarshal(v, c)

	default:
		return fmt.Errorf("type %T is not supported for cluster artifact files", value)
	}
}

type ClusterArtifacts []ClusterArtifact

type ClusterArtifactArchiveType struct {
	Ext        string
	MimeType   string
	Compressed bool
}

const (
	ClusterArtifactArchiveTypeExtZip = "zip"
)

var ClusterArtifactArchiveTypes = map[string]ClusterArtifactArchiveType{
	ClusterArtifactArchiveTypeExtZip: {
		Ext:        ClusterArtifactArchiveTypeExtZip,
		MimeType:   "application/zip",
		Compressed: true,
	},
}
