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

type NetworkPeer struct {
	ID          int
	UUID        uuid.UUID
	Cluster     string
	NetworkName string
	Name        string
	Object      incusapi.NetworkPeer
	LastUpdated time.Time
}

func (m *NetworkPeer) DeriveUUID() *NetworkPeer {
	identifier := strings.Join([]string{
		m.Cluster,
		m.NetworkName,
		m.Name,
	}, ":")

	m.UUID = uuid.NewSHA1(InventorySpaceUUID, []byte(identifier))

	return m
}

func (m NetworkPeer) Validate() error {
	if m.Cluster == "" {
		return domain.NewValidationErrf("Invalid NetworkPeer, cluster can not be empty")
	}

	if m.Name == "" {
		return domain.NewValidationErrf("Invalid NetworkPeer, name can not be empty")
	}

	if m.NetworkName == "" {
		return domain.NewValidationErrf("Invalid NetworkPeer, parent name (Network) can not be empty")
	}

	clone := m
	clone.DeriveUUID()
	if clone.UUID != m.UUID {
		return domain.NewValidationErrf("Invalid UUID, does not match derived value")
	}

	return nil
}

type NetworkPeers []NetworkPeer

type NetworkPeerFilter struct {
	Cluster    *string
	Expression *string
}

func (f NetworkPeerFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Cluster != nil {
		query.Add("cluster", *f.Cluster)
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f NetworkPeerFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}
