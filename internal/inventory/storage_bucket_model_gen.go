// Code generated by generate-inventory; DO NOT EDIT.

package inventory

import (
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
)

type StorageBucket struct {
	ID              int
	UUID            uuid.UUID
	Cluster         string
	Server          string
	ProjectName     string
	StoragePoolName string
	Name            string
	Object          incusapi.StorageBucket
	LastUpdated     time.Time
}

func (m *StorageBucket) DeriveUUID() *StorageBucket {
	identifier := strings.Join([]string{
		m.Cluster,
		m.Server,
		m.ProjectName,
		m.StoragePoolName,
		m.Name,
	}, ":")

	m.UUID = uuid.NewSHA1(InventorySpaceUUID, []byte(identifier))

	return m
}

func (m StorageBucket) Validate() error {
	if m.Cluster == "" {
		return domain.NewValidationErrf("Invalid StorageBucket, cluster can not be empty")
	}

	if m.Server == "" {
		return domain.NewValidationErrf("Invalid StorageBucket, server can not be empty")
	}

	if m.Name == "" {
		return domain.NewValidationErrf("Invalid StorageBucket, name can not be empty")
	}

	if m.ProjectName == "" {
		return domain.NewValidationErrf("Invalid StorageBucket, project name can not be empty")
	}

	if m.StoragePoolName == "" {
		return domain.NewValidationErrf("Invalid StorageBucket, parent name (StoragePool) can not be empty")
	}

	clone := m
	clone.DeriveUUID()
	if clone.UUID != m.UUID {
		return domain.NewValidationErrf("Invalid UUID, does not match derived value")
	}

	return nil
}

type StorageBuckets []StorageBucket

type StorageBucketFilter struct {
	Cluster    *string
	Server     *string
	Project    *string
	Expression *string
}

func (f StorageBucketFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Cluster != nil {
		query.Add("cluster", *f.Cluster)
	}

	if f.Server != nil {
		query.Add("server", *f.Server)
	}

	if f.Project != nil {
		query.Add("project", *f.Project)
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f StorageBucketFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}
