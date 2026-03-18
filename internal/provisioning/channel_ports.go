package provisioning

import (
	"context"

	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/shared/api"
)

type ChannelService interface {
	SetServerService(serverSvc ServerService)

	Create(ctx context.Context, newChannel Channel) (Channel, error)
	GetAll(ctx context.Context) (Channels, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Channel, error)
	Update(ctx context.Context, newChannel Channel) error
	DeleteByName(ctx context.Context, name string) error
	GetChangelogByName(ctx context.Context, name string, architecture images.UpdateFileArchitecture) (api.UpdateChangelogs, error)
}

type ChannelRepo interface {
	Create(ctx context.Context, newChannel Channel) (int64, error)
	GetAll(ctx context.Context) (Channels, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Channel, error)
	Update(ctx context.Context, newChannel Channel) error
	DeleteByName(ctx context.Context, name string) error
}
