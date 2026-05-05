package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

type createMocks struct {
	repo      *repoMock.ServerRepoMock
	client    *adapterMock.ServerClientPortMock
	tokenSvc  *svcMock.TokenServiceMock
	updateSvc *svcMock.UpdateServiceMock
}

func initCreateMocks(t *testing.T, setup func(*createMocks)) (_ *createMocks, fixedDate time.Time) {
	t.Helper()

	fixedDate = time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	m := &createMocks{
		repo: &repoMock.ServerRepoMock{
			CreateFunc: func(ctx context.Context, in provisioning.Server) (int64, error) {
				require.Equal(t, fixedDate, in.LastSeen)
				return 1, nil
			},
			GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
				return &provisioning.Server{}, nil
			},
			UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
				return nil
			},
		},

		client: &adapterMock.ServerClientPortMock{
			PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
				return nil
			},
			IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
				return nil
			},
			GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
				return api.HardwareData{}, nil
			},
			GetOSDataFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
				return api.OSData{
					Network: incusosapi.SystemNetwork{
						State: incusosapi.SystemNetworkState{
							Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
								"eth0": {
									Addresses: []string{"192.168.0.100"},
									Roles:     []string{"management"},
								},
							},
						},
					},
				}, nil
			},
			GetVersionDataFunc: func(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
				return api.ServerVersionData{}, nil
			},
			GetServerTypeFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
				return api.ServerTypeIncus, nil
			},
		},

		tokenSvc: &svcMock.TokenServiceMock{
			ConsumeFunc: func(ctx context.Context, id uuid.UUID) (string, error) {
				return "stable", nil
			},
		},

		updateSvc: &svcMock.UpdateServiceMock{
			GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
				return provisioning.Updates{}, nil
			},
		},
	}

	if setup != nil {
		setup(m)
	}

	return m, fixedDate
}
