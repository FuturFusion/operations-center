package provisioning

import (
	"context"
)

type ChannelService interface {
	SetServerService(serverSvc ServerService)

	Create(ctx context.Context, newChannel Channel) (Channel, error)
	GetAll(ctx context.Context) (Channels, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Channel, error)
	Update(ctx context.Context, newChannel Channel) error
	DeleteByName(ctx context.Context, name string) error
}

type ChannelRepo interface {
	Create(ctx context.Context, newChannel Channel) (int64, error)
	GetAll(ctx context.Context) (Channels, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*Channel, error)
	Update(ctx context.Context, newChannel Channel) error
	DeleteByName(ctx context.Context, name string) error
}
