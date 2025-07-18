// Code generated by generate-inventory; DO NOT EDIT.

package inventory

import (
	"context"

	"github.com/google/uuid"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type NetworkAddressSetService interface {
	GetAllWithFilter(ctx context.Context, filter NetworkAddressSetFilter) (NetworkAddressSets, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter NetworkAddressSetFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (NetworkAddressSet, error)
	ResyncByUUID(ctx context.Context, id uuid.UUID) error
	SyncCluster(ctx context.Context, cluster string) error
}

type NetworkAddressSetRepo interface {
	GetAllWithFilter(ctx context.Context, filter NetworkAddressSetFilter) (NetworkAddressSets, error)
	GetAllUUIDsWithFilter(ctx context.Context, filter NetworkAddressSetFilter) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (NetworkAddressSet, error)
	Create(ctx context.Context, networkAddressSet NetworkAddressSet) (NetworkAddressSet, error)
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	DeleteByClusterName(ctx context.Context, cluster string) error
	UpdateByUUID(ctx context.Context, networkAddressSet NetworkAddressSet) (NetworkAddressSet, error)
}

type NetworkAddressSetServerClient interface {
	HasExtension(ctx context.Context, cluster provisioning.Cluster, extension string) (exists bool)
	GetNetworkAddressSets(ctx context.Context, cluster provisioning.Cluster) ([]incusapi.NetworkAddressSet, error)
	GetNetworkAddressSetByName(ctx context.Context, cluster provisioning.Cluster, networkAddressSetName string) (incusapi.NetworkAddressSet, error)
}
