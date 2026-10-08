package server_test

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"testing"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerService_UpdateSystemNetwork(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	type repoUpdateFuncItem struct {
		lastSeen time.Time
		status   api.ServerStatus
	}

	tests := []struct {
		name                         string
		ctx                          context.Context
		repoGetByNameServer          provisioning.Server
		repoGetByNameErr             error
		repoUpdate                   []queue.Item[repoUpdateFuncItem]
		clientUpdateNetworkConfigErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			ctx:              t.Context(),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.UpdateByID",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateNetworkConfig with cancelled context with cause",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancelCause(t.Context())
				cancel(nil)
				return ctx
			}(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, context.Canceled)
			},
		},
		{
			name: "error - client.UpdateNetworkConfig",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},
			clientUpdateNetworkConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateNetworkConfig - reverter error",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
					Err: errors.New("reverter"),
				},
			},
			clientUpdateNetworkConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					value, err := queue.Pop(t, &tc.repoUpdate)

					require.Equal(t, value.lastSeen, in.LastSeen)
					require.Equal(t, value.status, in.Status)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateNetworkConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					return errors.Join(tc.clientUpdateNetworkConfigErr, ctx.Err())
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			// Register our own self update signal, such that we can ensure, that all the listeners
			// have been removed after successful processing.
			selfUpdateSignal := signals.New[provisioning.Server]()

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithSelfUpdateSignal(selfUpdateSignal),
			)

			// Run test
			err := serverSvc.UpdateSystemNetwork(tc.ctx, "one", provisioning.ServerSystemNetwork{})

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
			require.True(t, selfUpdateSignal.IsEmpty())
		})
	}
}

func TestServerService_UpdateSystemStorage(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	type repoUpdateFuncItem struct {
		lastSeen time.Time
		status   api.ServerStatus
	}

	tests := []struct {
		name                         string
		ctx                          context.Context
		repoGetByNameServer          provisioning.Server
		repoGetByNameErr             error
		repoUpdate                   []queue.Item[repoUpdateFuncItem]
		clientUpdateStorageConfigErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			ctx:              t.Context(),
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.UpdateByID",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateStorageConfig with cancelled context with cause",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancelCause(t.Context())
				cancel(nil)
				return ctx
			}(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, context.Canceled)
			},
		},
		{
			name: "error - client.UpdateStorageConfig",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
				},
			},
			clientUpdateStorageConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateStorageConfig - reverter error",
			ctx:  t.Context(),
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
				{
					Value: repoUpdateFuncItem{
						status: api.ServerStatusReady,
					},
					Err: errors.New("reverter"),
				},
			},
			clientUpdateStorageConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					value, err := queue.Pop(t, &tc.repoUpdate)

					require.Equal(t, value.lastSeen, in.LastSeen)
					require.Equal(t, value.status, in.Status)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateStorageConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					return errors.Join(tc.clientUpdateStorageConfigErr, ctx.Err())
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
			)

			// Run test
			err := serverSvc.UpdateSystemStorage(tc.ctx, "one", provisioning.ServerSystemStorage{})

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
		})
	}
}

func TestServerService_GetSystemProvider(t *testing.T) {
	tests := []struct {
		name                       string
		repoGetByNameServer        provisioning.Server
		repoGetByNameErr           error
		clientGetProviderConfig    provisioning.ServerSystemProvider
		clientGetProviderConfigErr error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemProvider
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetProviderConfig: provisioning.ServerSystemProvider{
				Config: incusosapi.SystemProviderConfig{
					Name: "operations-center",
				},
				State: incusosapi.SystemProviderState{
					Registered: true,
				},
			},

			assertErr: require.NoError,
			want: provisioning.ServerSystemProvider{
				Config: incusosapi.SystemProviderConfig{
					Name: "operations-center",
				},
				State: incusosapi.SystemProviderState{
					Registered: true,
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetProviderConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetProviderConfigFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemProvider, error) {
					return tc.clientGetProviderConfig, tc.clientGetProviderConfigErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			got, err := serverSvc.GetSystemProvider(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_UpdateSystemProvider(t *testing.T) {
	tests := []struct {
		name                          string
		repoGetByNameServer           provisioning.Server
		repoGetByNameErr              error
		clientUpdateProviderConfigErr error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemProvider
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateProviderConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status: api.ServerStatusReady,
			},
			clientUpdateProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateProviderConfigFunc: func(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemProvider) error {
					return tc.clientUpdateProviderConfigErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemProvider(
				t.Context(), "one", incusosapi.SystemProvider{
					Config: incusosapi.SystemProviderConfig{
						Name: "operations-center-new",
					},
				},
			)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_GetSystemUpdate(t *testing.T) {
	tests := []struct {
		name                     string
		repoGetByNameServer      provisioning.Server
		repoGetByNameErr         error
		clientGetUpdateConfig    provisioning.ServerSystemUpdate
		clientGetUpdateConfigErr error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemUpdate
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfig: provisioning.ServerSystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},

			assertErr: require.NoError,
			want: provisioning.ServerSystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.GetUpdateConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetUpdateConfigFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemUpdate, error) {
					return tc.clientGetUpdateConfig, tc.clientGetUpdateConfigErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			got, err := serverSvc.GetSystemUpdate(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_UpdateSystemUpdate(t *testing.T) {
	tests := []struct {
		name                        string
		repoGetByNameServer         provisioning.Server
		repoGetByNameErr            error
		clientGetUpdateConfig       provisioning.ServerSystemUpdate
		clientUpdateUpdateConfigErr error
		channelSvcGetByNameErr      error

		assertErr require.ErrorAssertionFunc
		want      provisioning.ServerSystemUpdate
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfig: incusosapi.SystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                   "error - updateSvc.GetChannelByName",
			channelSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - client.UpdateUpdateConfig",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
		one
		-----END CERTIFICATE-----
		`),
				Status: api.ServerStatusReady,
			},
			clientGetUpdateConfig: incusosapi.SystemUpdate{
				Config: incusosapi.SystemUpdateConfig{
					AutoReboot:     false,
					Channel:        "stable",
					CheckFrequency: "6h",
				},
				State: incusosapi.SystemUpdateState{
					NeedsReboot: false,
					LastCheck:   time.Date(2026, 1, 13, 16, 13, 47, 0, time.UTC),
					Status:      "Update check completed",
				},
			},
			clientUpdateUpdateConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
					return nil
				},
				IsReadyFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
				GetResourcesFunc: func(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
					return api.HardwareData{}, boom.Error // Since we do not care too much, if the server poll was successful, we always return an error here.
				},
				UpdateUpdateConfigFunc: func(ctx context.Context, server provisioning.Server, updateConfig provisioning.ServerSystemUpdate) error {
					require.False(t, updateConfig.Config.AutoReboot)              // AutoReboot is forced to false.
					require.Equal(t, "never", updateConfig.Config.CheckFrequency) // CheckFrequency is forced to "never".
					return tc.clientUpdateUpdateConfigErr
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return &provisioning.Channel{}, tc.channelSvcGetByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, channelSvc, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemUpdate(
				t.Context(), "one", incusosapi.SystemUpdate{
					Config: incusosapi.SystemUpdateConfig{
						AutoReboot:     true,
						Channel:        "testing",
						CheckFrequency: "2h",
					},
				},
			)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_UpdateSystemNetworkWithSelfUpdateSignal(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	type repoUpdateFuncItem struct {
		lastSeen time.Time
		status   api.ServerStatus
	}

	tests := []struct {
		name                         string
		repoGetByNameServer          provisioning.Server
		repoGetByNameErr             error
		repoUpdate                   []queue.Item[repoUpdateFuncItem]
		clientUpdateNetworkConfigErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByNameServer: provisioning.Server{
				Name:          "one",
				Type:          api.ServerTypeIncus,
				Cluster:       new("one"),
				ConnectionURL: "http://one/",
				Certificate: new(`-----BEGIN CERTIFICATE-----
one
-----END CERTIFICATE-----
`),
				Status:  api.ServerStatusReady,
				Channel: "stable",
			},
			repoUpdate: []queue.Item[repoUpdateFuncItem]{
				{
					Value: repoUpdateFuncItem{
						lastSeen: fixedDate,
						status:   api.ServerStatusPending,
					},
				},
			},

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					value, err := queue.Pop(t, &tc.repoUpdate)

					require.Equal(t, value.lastSeen, in.LastSeen)
					require.Equal(t, value.status, in.Status)
					return err
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateNetworkConfigFunc: func(ctx context.Context, server provisioning.Server) error {
					// Simulate network change, which prevents a clean response.
					<-ctx.Done()
					return ctx.Err()
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			selfUpdateSignal := signals.New[provisioning.Server]()

			serverSvc := provisioningServer.New(
				repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.WithSelfUpdateSignal(selfUpdateSignal),
			)

			// Run test
			wg := sync.WaitGroup{}
			wg.Add(1)

			var err error
			go func() {
				defer wg.Done()

				err = serverSvc.UpdateSystemNetwork(t.Context(), "one", provisioning.ServerSystemNetwork{})
			}()

			// Wait for subscriber.
			for selfUpdateSignal.IsEmpty() {
				time.Sleep(time.Millisecond)
			}

			// Simulate update from a different node, which is ignored.
			selfUpdateSignal.Emit(t.Context(), provisioning.Server{
				Name: "another",
			})

			selfUpdateSignal.Emit(t.Context(), provisioning.Server{
				Name: "one",
			})

			wg.Wait()

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoUpdate)
			require.True(t, selfUpdateSignal.IsEmpty())
		})
	}
}

func TestServerService_GetSystemLogging(t *testing.T) {
	tests := []struct {
		name                      string
		argName                   string
		repoGetByName             *provisioning.Server
		repoGetByNameErr          error
		clientGetSystemLogging    provisioning.ServerSystemLogging
		clientGetSystemLoggingErr error

		assertErr         require.ErrorAssertionFunc
		wantLoggingConfig provisioning.ServerSystemLogging
	}{
		{
			name:    "success",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemLogging: incusosapi.SystemLogging{
				Config: incusosapi.SystemLoggingConfig{
					Syslog: incusosapi.SystemLoggingSyslog{
						Address: "localhost",
					},
				},
			},

			assertErr: require.NoError,
			wantLoggingConfig: incusosapi.SystemLogging{
				Config: incusosapi.SystemLoggingConfig{
					Syslog: incusosapi.SystemLoggingSyslog{
						Address: "localhost",
					},
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.GetSystemLogging",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemLoggingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetSystemLoggingFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemLogging, error) {
					return tc.clientGetSystemLogging, tc.clientGetSystemLoggingErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			loggingConfig, err := serverSvc.GetSystemLogging(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantLoggingConfig, loggingConfig)
		})
	}
}

func TestServerService_UpdateSystemLogging(t *testing.T) {
	tests := []struct {
		name                         string
		argName                      string
		argLoggingConfig             incusosapi.SystemLogging
		repoGetByName                *provisioning.Server
		repoGetByNameErr             error
		clientUpdateSystemLoggingErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:             "success",
			argName:          "one",
			argLoggingConfig: incusosapi.SystemLogging{},
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.UpdateSystemLogging",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientUpdateSystemLoggingErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateSystemLoggingFunc: func(ctx context.Context, server provisioning.Server, config provisioning.ServerSystemLogging) error {
					return tc.clientUpdateSystemLoggingErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemLogging(t.Context(), tc.argName, tc.argLoggingConfig)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_GetSystemKernel(t *testing.T) {
	tests := []struct {
		name                     string
		argName                  string
		repoGetByName            *provisioning.Server
		repoGetByNameErr         error
		clientGetSystemKernel    provisioning.ServerSystemKernel
		clientGetSystemKernelErr error

		assertErr        require.ErrorAssertionFunc
		wantKernelConfig provisioning.ServerSystemKernel
	}{
		{
			name:    "success",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemKernel: incusosapi.SystemKernel{
				Config: incusosapi.SystemKernelConfig{
					BlacklistModules: []string{"foobar"},
				},
			},

			assertErr: require.NoError,
			wantKernelConfig: incusosapi.SystemKernel{
				Config: incusosapi.SystemKernelConfig{
					BlacklistModules: []string{"foobar"},
				},
			},
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.GetSystemKernel",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientGetSystemKernelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				GetSystemKernelFunc: func(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemKernel, error) {
					return tc.clientGetSystemKernel, tc.clientGetSystemKernelErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			kernelConfig, err := serverSvc.GetSystemKernel(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantKernelConfig, kernelConfig)
		})
	}
}

func TestServerService_UpdateSystemKernel(t *testing.T) {
	tests := []struct {
		name                        string
		argName                     string
		argKernelConfig             incusosapi.SystemKernel
		repoGetByName               *provisioning.Server
		repoGetByNameErr            error
		clientUpdateSystemKernelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "success",
			argName:         "one",
			argKernelConfig: incusosapi.SystemKernel{},
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.UpdateSystemKernel",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientUpdateSystemKernelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				UpdateSystemKernelFunc: func(ctx context.Context, server provisioning.Server, config provisioning.ServerSystemKernel) error {
					return tc.clientUpdateSystemKernelErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.UpdateSystemKernel(t.Context(), tc.argName, tc.argKernelConfig)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_AddApplication(t *testing.T) {
	tests := []struct {
		name                    string
		argName                 string
		argApplicationName      string
		repoGetByName           *provisioning.Server
		repoGetByNameErr        error
		clientAddApplicationErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success",
			argName:            "one",
			argApplicationName: "debug",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.AddApplication",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientAddApplicationErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				AddApplicationFunc: func(ctx context.Context, server provisioning.Server, application string) error {
					return tc.clientAddApplicationErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.AddApplication(t.Context(), tc.argName, tc.argApplicationName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_RestartApplication(t *testing.T) {
	tests := []struct {
		name                        string
		argName                     string
		argApplicationName          string
		repoGetByName               *provisioning.Server
		repoGetByNameErr            error
		clientRestartApplicationErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:               "success",
			argName:            "one",
			argApplicationName: "debug",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			argName:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - client.RestartApplication",
			argName: "one",
			repoGetByName: &provisioning.Server{
				Channel: "stable",
			},
			clientRestartApplicationErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			client := &adapterMock.ServerClientPortMock{
				RestartApplicationFunc: func(ctx context.Context, server provisioning.Server, application string) error {
					return tc.clientRestartApplicationErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return provisioning.Updates{}, nil
				},
			}

			serverSvc := provisioningServer.New(repo, client, nil, nil, nil, nil, updateSvc, tls.Certificate{})

			// Run test
			err := serverSvc.RestartApplication(t.Context(), tc.argName, tc.argApplicationName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
