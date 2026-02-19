package provisioning

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestChannelService_validateUpdatesConfig(t *testing.T) {
	tests := []struct {
		name                  string
		updatesDefaultChannel string
		serverDefaultChannel  string
		repoGetByName         []queue.Item[struct{}]

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                  "success",
			updatesDefaultChannel: "stable",
			serverDefaultChannel:  "stable",
			repoGetByName: []queue.Item[struct{}]{
				{},
				{},
			},

			assertErr: require.NoError,
		},
		{
			name:                  "error - update default channel empty",
			updatesDefaultChannel: "", // empty
			serverDefaultChannel:  "stable",
			repoGetByName:         []queue.Item[struct{}]{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "updates.updates_default_channel" can not be empty`)
			},
		},
		{
			name:                  "error - update default channel not present",
			updatesDefaultChannel: "not present",
			serverDefaultChannel:  "stable",
			repoGetByName: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, failed to get "updates.updates_default_channel":`)
			},
		},
		{
			name:                  "error - server default channel empty",
			updatesDefaultChannel: "stable",
			serverDefaultChannel:  "", // empty
			repoGetByName: []queue.Item[struct{}]{
				{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "updates.server_default_channel" can not be empty`)
			},
		},
		{
			name:                  "error - server default channel not present",
			updatesDefaultChannel: "stable",
			serverDefaultChannel:  "not present",
			repoGetByName: []queue.Item[struct{}]{
				{},
				{
					Err: boom.Error,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, failed to get "updates.server_default_channel":`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*Channel, error) {
					_, err := queue.Pop(t, &tc.repoGetByName)
					return nil, err
				},
			}

			channelSvc := NewChannelService(repo, nil)

			err := channelSvc.validateUpdatesConfig(t.Context(), api.SystemUpdates{
				SystemUpdatesPut: api.SystemUpdatesPut{
					UpdatesDefaultChannel: tc.updatesDefaultChannel,
					ServerDefaultChannel:  tc.serverDefaultChannel,
				},
			})

			tc.assertErr(t, err)
			require.Empty(t, tc.repoGetByName)
		})
	}
}

// Can not use the regular generated mock here, since it produces a circular dependency.
type repoMock struct {
	CreateFunc       func(ctx context.Context, newChannel Channel) (int64, error)
	GetAllFunc       func(ctx context.Context) (Channels, error)
	GetAllNamesFunc  func(ctx context.Context) ([]string, error)
	GetByNameFunc    func(ctx context.Context, name string) (*Channel, error)
	UpdateFunc       func(ctx context.Context, newChannel Channel) error
	DeleteByNameFunc func(ctx context.Context, name string) error
}

func (r repoMock) Create(ctx context.Context, newChannel Channel) (int64, error) {
	if r.CreateFunc == nil {
		panic("not implemented")
	}

	return r.CreateFunc(ctx, newChannel)
}

func (r repoMock) GetAll(ctx context.Context) (Channels, error) {
	if r.GetAllFunc == nil {
		panic("not implemented")
	}

	return r.GetAllFunc(ctx)
}

func (r repoMock) GetAllNames(ctx context.Context) ([]string, error) {
	if r.GetAllNamesFunc == nil {
		panic("not implemented")
	}

	return r.GetAllNamesFunc(ctx)
}

func (r repoMock) GetByName(ctx context.Context, name string) (*Channel, error) {
	if r.GetByNameFunc == nil {
		panic("not implemented")
	}

	return r.GetByNameFunc(ctx, name)
}

func (r repoMock) Update(ctx context.Context, newChannel Channel) error {
	if r.UpdateFunc == nil {
		panic("not implemented")
	}

	return r.UpdateFunc(ctx, newChannel)
}

func (r repoMock) DeleteByName(ctx context.Context, name string) error {
	if r.DeleteByNameFunc == nil {
		panic("not implemented")
	}

	return r.DeleteByNameFunc(ctx, name)
}
