package server_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

const testSeedImageID = "a1B2c3D4e5F6"

func TestServerService_ResyncBMCData(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name string

		repoGetAllServers provisioning.Servers
		repoGetAllErr     error

		bmcClientGetData    api.BMCData
		bmcClientGetDataErr error

		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error
		repoUpdateErr       error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - no servers",

			assertErr: require.NoError,
		},
		{
			name: "success - server without BMC type configured",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeNone,
						Endpoint: "https://bmc.local",
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - server with BMC type but no endpoint",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name: "success - resync",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name:          "error - repo.GetAll",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - no BMC server client registered for type",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPIType("unknown"),
						Endpoint: "https://bmc.local",
					},
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name: "error - client.GetServerDetails",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetDataErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.GetByName",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "success - upper case server UUID reported by BMC",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "E9DE436E-B94E-4AEF-8563-883AEC84096E",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - repo.Update",
			repoGetAllServers: provisioning.Servers{
				{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType:  api.BMCAPITypeRedfishV1Generic,
						Endpoint: "https://bmc.local",
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.repoGetAllServers, tc.repoGetAllErr
				},
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					// What the BMC reported is stored as it is, except for the
					// parts it could not report, which keep what was observed of
					// them before.
					wantDetails := tc.bmcClientGetData.CarryOver(tc.repoGetByNameServer.BMCData)
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData, "BMC data should be stored as reported by the BMC")
					require.Equal(t, new(strings.ToLower(wantDetails.ServerUUID)), in.SystemUUID, "system UUID should be stored in lower case")

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.ResyncBMCData(t.Context())

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCServerPowerOnByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                      string
		nameArg                   string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		bmcClientServerPowerOnErr error
		bmcClientWaitErr          error
		bmcClientGetData          api.BMCData
		bmcClientGetDataErr       error
		repoUpdateErr             error
		resyncDone                chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - task monitor wait fails but resync still runs",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - resync fails",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,
			resyncDone:          make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:       "error - name empty",
			nameArg:    "", // invalid
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,
			resyncDone:       closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ServerPowerOn",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerPowerOnErr: boom.Error,
			resyncDone:                closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerPowerOnFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
					return taskMonitor, tc.bmcClientServerPowerOnErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerPowerOnByName(t.Context(), tc.nameArg, false)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}

func TestServerService_BMCServerPowerOffByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                       string
		nameArg                    string
		repoGetByNameServer        *provisioning.Server
		repoGetByNameErr           error
		bmcClientServerPowerOffErr error
		bmcClientWaitErr           error
		bmcClientGetData           api.BMCData
		bmcClientGetDataErr        error
		repoUpdateErr              error
		resyncDone                 chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - task monitor wait fails but resync still runs",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:    "success - resync fails",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,
			resyncDone:          make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:       "error - name empty",
			nameArg:    "", // invalid
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,
			resyncDone:       closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ServerPowerOff",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerPowerOffErr: boom.Error,
			resyncDone:                 closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerPowerOffFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
					return taskMonitor, tc.bmcClientServerPowerOffErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerPowerOffByName(t.Context(), tc.nameArg, false)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}

func TestServerService_BMCServerRestartByName(t *testing.T) {
	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	tests := []struct {
		name                      string
		nameArg                   string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		bmcClientServerRestartErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ServerRestart",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerRestartErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerRestartFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
					return taskMonitor, tc.bmcClientServerRestartErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerRestartByName(t.Context(), tc.nameArg, false)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCServerSetLocationIndicatorByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name                                   string
		nameArg                                string
		activeArg                              bool
		repoGetByNameServer                    *provisioning.Server
		repoGetByNameErr                       error
		bmcClientServerSetLocationIndicatorErr error
		bmcClientGetData                       api.BMCData
		bmcClientGetDataErr                    error
		repoUpdateErr                          error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:      "success - turn on",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerLocationIndicatorActive: true,
			},

			assertErr: require.NoError,
		},
		{
			name:      "success - turn off",
			nameArg:   "one",
			activeArg: false,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetData: api.BMCData{
				ServerLocationIndicatorActive: false,
			},

			assertErr: require.NoError,
		},
		{
			name:      "success - resync fails",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,

			assertErr: require.NoError,
		},
		{
			name:      "success - repo.Update fails during resync",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			repoUpdateErr: boom.Error,

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:      "error - client.ServerSetLocationIndicator",
			nameArg:   "one",
			activeArg: true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientServerSetLocationIndicatorErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					wantDetails := tc.bmcClientGetData
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ServerSetLocationIndicatorFunc: func(ctx context.Context, server provisioning.Server, active bool) error {
					require.Equal(t, tc.activeArg, active)

					return tc.bmcClientServerSetLocationIndicatorErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCServerSetLocationIndicatorByName(t.Context(), tc.nameArg, tc.activeArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_ApplyBIOSAttributesByName(t *testing.T) {
	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                            string
		nameArg                         string
		attributesArg                   map[string]any
		repoGetByNameServer             *provisioning.Server
		repoGetByNameErr                error
		bmcClientApplyBIOSAttributesErr error
		bmcClientWaitErr                error
		waitDone                        chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success",
			nameArg:       "one",
			attributesArg: map[string]any{"SecureBoot": "Enabled"},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			waitDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:          "success - wait for task fails",
			nameArg:       "one",
			attributesArg: map[string]any{"SecureBoot": "Enabled"},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			waitDone:         make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:     "error - name empty",
			nameArg:  "", // invalid
			waitDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,
			waitDone:         closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			waitDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:          "error - client.ApplyBIOSAttributes",
			nameArg:       "one",
			attributesArg: map[string]any{"SecureBoot": "Enabled"},
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientApplyBIOSAttributesErr: boom.Error,
			waitDone:                        closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ApplyBIOSAttributesFunc: func(ctx context.Context, server provisioning.Server, attributes map[string]any) (*provisioning.BMCTaskMonitor, error) {
					require.Equal(t, tc.attributesArg, attributes)

					return taskMonitor, tc.bmcClientApplyBIOSAttributesErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					defer close(tc.waitDone)

					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.ApplyBIOSAttributesByName(t.Context(), tc.nameArg, tc.attributesArg)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.waitDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC task wait")
			}
		})
	}
}

func TestServerService_BMCBIOSAttributesByName(t *testing.T) {
	attributes := []api.BIOSAttribute{
		{Name: "NumaNodesPerSocket", Type: "String", CurrentValue: "4"},
		{Name: "SecureBoot", Type: "Enumeration", CurrentValue: "Enabled"},
	}

	tests := []struct {
		name                       string
		nameArg                    string
		repoGetByNameServer        *provisioning.Server
		repoGetByNameErr           error
		bmcClientBIOSAttributesErr error

		assertErr require.ErrorAssertionFunc
		want      []api.BIOSAttribute
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      attributes,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.BIOSAttributes",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientBIOSAttributesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
					return attributes, tc.bmcClientBIOSAttributesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			got, err := serverSvc.BMCBIOSAttributesByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_BIOSProfileByName(t *testing.T) {
	resolution := provisioning.BIOSProfileResolution{
		Profiles: []string{"dell-poweredge"},
		Attributes: map[string]any{
			"SecureBoot": "Enabled",
		},
	}

	tests := []struct {
		name                   string
		nameArg                string
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		biosProfileResolve     *provisioning.BIOSProfileResolution
		biosProfileResolveErr  error
		withoutBIOSProfilePort bool

		assertErr require.ErrorAssertionFunc
		want      *provisioning.BIOSProfileResolution
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			biosProfileResolve: &resolution,

			assertErr: require.NoError,
			want:      &resolution,
		},
		{
			name:    "success - no profile matches",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BIOS profile port configured",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			withoutBIOSProfilePort: true,

			assertErr: errassert.NotFoundError,
		},
		{
			name:    "error - biosProfile.Resolve",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
			},
			biosProfileResolveErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			opts := []provisioningServer.Option{}
			if !tc.withoutBIOSProfilePort {
				opts = append(opts, provisioningServer.WithBIOSProfilePort(&adapterMock.BIOSProfilePortMock{
					ResolveFunc: func(ctx context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
						return tc.biosProfileResolve, tc.biosProfileResolveErr
					},
				}))
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				opts...,
			)

			// Run test
			got, err := serverSvc.BIOSProfileByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_ValidateBIOSProfileByName(t *testing.T) {
	resolution := provisioning.BIOSProfileResolution{
		Profiles: []string{"dell-poweredge"},
		Attributes: map[string]any{
			"SecureBoot": "Enabled",
		},
	}

	tests := []struct {
		name                       string
		nameArg                    string
		repoGetByNameServer        *provisioning.Server
		biosProfileResolve         *provisioning.BIOSProfileResolution
		bmcClientBIOSAttributes    []api.BIOSAttribute
		bmcClientBIOSAttributesErr error

		assertErr require.ErrorAssertionFunc
		want      *provisioning.BIOSProfileResolution
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			biosProfileResolve: &resolution,
			bmcClientBIOSAttributes: []api.BIOSAttribute{
				{Name: "SecureBoot", Type: "Enumeration", AcceptableValues: []string{"Enabled", "Disabled"}},
			},

			assertErr: require.NoError,
			want:      &resolution,
		},
		{
			name:    "success - no profile matches",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:    "error - attribute not accepted by the BMC",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			biosProfileResolve: &resolution,
			bmcClientBIOSAttributes: []api.BIOSAttribute{
				{Name: "SecureBoot", Type: "Enumeration", AcceptableValues: []string{"Disabled"}},
			},

			assertErr: errassert.ValidationErrorContains(`"SecureBoot"`),
		},
		{
			name:    "error - client.BIOSAttributes",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			biosProfileResolve:         &resolution,
			bmcClientBIOSAttributesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, nil
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
					return tc.bmcClientBIOSAttributes, tc.bmcClientBIOSAttributesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
				provisioningServer.WithBIOSProfilePort(&adapterMock.BIOSProfilePortMock{
					ResolveFunc: func(ctx context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
						return tc.biosProfileResolve, nil
					},
				}),
			)

			// Run test
			got, err := serverSvc.ValidateBIOSProfileByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_BMCBIOSAttributeAcceptableValuesByNameAndAttribute(t *testing.T) {
	values := api.BIOSAttribute{
		CurrentValue:     "Enabled",
		AcceptableValues: []string{"Enabled", "Disabled"},
	}

	tests := []struct {
		name                      string
		nameArg                   string
		attributeNameArg          string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		bmcClientBIOSAttributeErr error

		assertErr require.ErrorAssertionFunc
		want      api.BIOSAttribute
	}{
		{
			name:             "success",
			nameArg:          "one",
			attributeNameArg: "SecureBoot",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      values,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:             "error - client.BIOSAttribute",
			nameArg:          "one",
			attributeNameArg: "SecureBoot",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientBIOSAttributeErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				BIOSAttributeFunc: func(ctx context.Context, server provisioning.Server, attributeName string) (api.BIOSAttribute, error) {
					require.Equal(t, tc.attributeNameArg, attributeName)

					return values, tc.bmcClientBIOSAttributeErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			got, err := serverSvc.BMCBIOSAttributeByName(t.Context(), tc.nameArg, tc.attributeNameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestServerService_BMCApplySecureBootCertificatesByName(t *testing.T) {
	tests := []struct {
		name                                    string
		nameArg                                 string
		repoGetByNameServer                     *provisioning.Server
		repoGetByNameErr                        error
		bmcClientApplySecureBootCertificatesErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.ApplySecureBootCertificates",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientApplySecureBootCertificatesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				ApplySecureBootCertificatesFunc: func(ctx context.Context, server provisioning.Server, secureBoot api.BIOSSecureBoot) (bool, error) {
					return tc.bmcClientApplySecureBootCertificatesErr == nil, tc.bmcClientApplySecureBootCertificatesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCApplySecureBootCertificatesByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCLogSourcesByName(t *testing.T) {
	logSources := []string{"chassis/Logs", "manager/SEL", "system/Logs"}

	tests := []struct {
		name                   string
		nameArg                string
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		bmcClientLogSourcesErr error

		assertErr require.ErrorAssertionFunc
		want      []string
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      logSources,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.LogSources",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientLogSourcesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				LogSourcesFunc: func(ctx context.Context, server provisioning.Server) ([]string, error) {
					return logSources, tc.bmcClientLogSourcesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			gotLogSources, err := serverSvc.BMCLogSourcesByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, gotLogSources)
		})
	}
}

func TestServerService_BMCLogEntriesByNameAndLogSource(t *testing.T) {
	logEntries := []api.BMCLogEvent{
		{
			EntryType: "SEL",
			Message:   "A log message",
			Severity:  "OK",
		},
	}

	tests := []struct {
		name                   string
		nameArg                string
		logSourceArg           string
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		bmcClientLogEntriesErr error

		assertErr require.ErrorAssertionFunc
		want      []api.BMCLogEvent
	}{
		{
			name:         "success",
			nameArg:      "one",
			logSourceArg: "chassis/Logs",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      logEntries,
		},
		{
			name:         "error - name empty",
			nameArg:      "", // invalid
			logSourceArg: "chassis/Logs",

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:         "error - log source empty",
			nameArg:      "one",
			logSourceArg: "", // invalid

			assertErr: errassert.OperationNotPermittedErrorContains(`Log source "" must have the structure "service/logService"`),
		},
		{
			name:         "error - log source without log service",
			nameArg:      "one",
			logSourceArg: "chassis", // invalid

			assertErr: errassert.OperationNotPermittedErrorContains(`Log source "chassis" must have the structure "service/logService"`),
		},
		{
			name:         "error - log source with empty log service",
			nameArg:      "one",
			logSourceArg: "chassis/", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:         "error - log source with too many parts",
			nameArg:      "one",
			logSourceArg: "chassis/Logs/Entries", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			logSourceArg:     "chassis/Logs",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:         "error - no BMC server client registered for type",
			nameArg:      "one",
			logSourceArg: "chassis/Logs",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:         "error - client.LogEntriesBySource",
			nameArg:      "one",
			logSourceArg: "chassis/Logs",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientLogEntriesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				LogEntriesBySourceFunc: func(ctx context.Context, server provisioning.Server, logSource string) ([]api.BMCLogEvent, error) {
					return logEntries, tc.bmcClientLogEntriesErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			gotLogEntries, err := serverSvc.BMCLogEntriesByNameAndLogSource(t.Context(), tc.nameArg, tc.logSourceArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, gotLogEntries)
		})
	}
}

func TestServerService_BMCDumpByName(t *testing.T) {
	dump := api.BMCDump{
		"/redfish/v1/": api.BMCDumpEntry{
			Response: json.RawMessage(`{"Id":"RootService"}`),
		},
	}

	tests := []struct {
		name                   string
		nameArg                string
		additionalEndpointsArg []string
		skipPredefinedArg      bool
		traceArg               bool
		repoGetByNameServer    *provisioning.Server
		repoGetByNameErr       error
		bmcClientDumpErr       error

		assertErr require.ErrorAssertionFunc
		want      api.BMCDump
	}{
		{
			name:                   "success",
			nameArg:                "one",
			additionalEndpointsArg: []string{"/redfish/v1/Systems/1/Oem/Vendor"},
			skipPredefinedArg:      true,
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: require.NoError,
			want:      dump,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.Dump",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientDumpErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				DumpFunc: func(ctx context.Context, server provisioning.Server, additionalEndpoints []string, skipPredefined bool, trace bool) (api.BMCDump, error) {
					return dump, tc.bmcClientDumpErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			gotDump, err := serverSvc.BMCDumpByName(t.Context(), tc.nameArg, tc.additionalEndpointsArg, tc.skipPredefinedArg, tc.traceArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, gotDump)

			if tc.repoGetByNameServer != nil && tc.repoGetByNameServer.BMCConfig.APIType == api.BMCAPITypeRedfishV1Generic {
				require.Len(t, bmcClient.DumpCalls(), 1)
				require.Equal(t, tc.additionalEndpointsArg, bmcClient.DumpCalls()[0].AdditionalEndpoints)
				require.Equal(t, tc.skipPredefinedArg, bmcClient.DumpCalls()[0].SkipPredefined)
			}
		})
	}
}

func TestServerService_BMCRefreshByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	tests := []struct {
		name    string
		nameArg string

		repoGetByNameServer *provisioning.Server
		repoGetByNameErr    error

		bmcClientGetData    api.BMCData
		bmcClientGetDataErr error

		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},

			assertErr: require.NoError,
		},
		{
			name:    "success - upper case server UUID reported by BMC",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "E9DE436E-B94E-4AEF-8563-883AEC84096E",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:             "error - repo.GetByName",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPIType("unknown"),
					Endpoint: "https://bmc.local",
				},
			},

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.GetData",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetDataErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "success - a part, the BMC could not report, keeps what it last held",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
				BMCData: api.BMCData{
					ServerPowerState: "On",
					VirtualMedia: map[string]api.BMCVirtualMedia{
						"system:1": {ID: "system:1", Inserted: true},
					},
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID:       "e9de436e-b94e-4aef-8563-883aec84096e",
				ServerPowerState: "Off",
				Unavailable: map[api.BMCDataPart]string{
					api.BMCDataPartVirtualMedia: "BMC returned HTTP 503",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - repo.Update",
			nameArg: "one",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType:  api.BMCAPITypeRedfishV1Generic,
					Endpoint: "https://bmc.local",
				},
			},
			bmcClientGetData: api.BMCData{
				ServerUUID: "e9de436e-b94e-4aef-8563-883aec84096e",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					// What the BMC reported is stored as it is, except for the
					// parts it could not report, which keep what was observed of
					// them before.
					wantDetails := tc.bmcClientGetData.CarryOver(tc.repoGetByNameServer.BMCData)
					wantDetails.LastUpdated = fixedDate
					require.Equal(t, wantDetails, in.BMCData, "BMC data should be stored as reported by the BMC")
					require.Equal(t, new(strings.ToLower(wantDetails.ServerUUID)), in.SystemUUID, "system UUID should be stored in lower case")

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcClientGetData, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCRefreshByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestServerService_BMCAttachMediaByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	const tokenUUID = "e9de436e-b94e-4aef-8563-883aec84096e"

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	server := &provisioning.Server{
		Name: "one",
		BMCConfig: api.BMCConfig{
			APIType: api.BMCAPITypeRedfishV1Generic,
		},
	}

	tests := []struct {
		name                      string
		nameArg                   string
		mediaArg                  api.ServerBMCAttachMedia
		operationsCenterAddress   string
		repoGetByNameServer       *provisioning.Server
		repoGetByNameErr          error
		tokenSvcGetSeed           *provisioning.TokenSeed
		tokenSvcGetSeedErr        error
		tokenSvcResolveImageIDErr error
		channelSvcGetByNameErr    error
		bmcClientAttachMediaErr   error
		bmcClientWaitErr          error
		bmcClientGetDataErr       error
		repoUpdateErr             error
		resyncDone                chan struct{}

		wantMediaURL       string
		wantVirtualMediaID string
		assertErr          require.ErrorAssertionFunc
	}{
		{
			name:    "success - without channel",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - with boot device",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
				SetBootDevice:  true,
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - with channel",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "raw",
				Architecture:   "aarch64",
				Channel:        "stable",
				VirtualMediaID: "manager:2",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/aarch64/channel/stable/type/raw/" + testSeedImageID + ".raw",
			wantVirtualMediaID: "manager:2",
			assertErr:          require.NoError,
		},
		{
			name:    "success - channel needs URL escaping",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				Channel:        "team/beta 2",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/channel/team%2Fbeta%202/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - task monitor wait fails but resync still runs",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			bmcClientWaitErr:        boom.Error,
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - resync fails to get BMC data",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			bmcClientGetDataErr:     boom.Error,
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "success - resync fails to update the server",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			repoUpdateErr:           boom.Error,
			resyncDone:              make(chan struct{}),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          require.NoError,
		},
		{
			name:    "error - name empty",
			nameArg: "", // invalid
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - seed empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "", // invalid
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - virtual media ID empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				VirtualMediaID: "", // invalid
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - invalid token UUID",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      "not-a-uuid", // invalid
				Seed:           "default",
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:    "error - invalid image type",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "qcow2", // invalid
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid image type "qcow2"`),
		},
		{
			name:    "error - image type empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "", // invalid
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid image type ""`),
		},
		{
			name:    "error - invalid architecture",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "riscv64", // invalid
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid architecture "riscv64"`),
		},
		{
			name:    "error - architecture empty",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "", // invalid, undefined architecture
				VirtualMediaID: "system:1",
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Invalid architecture ""`),
		},
		{
			name:    "error - channel does not exist",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				Channel:        "does-not-exist",
				VirtualMediaID: "system:1",
			},
			channelSvcGetByNameErr: boom.Error,
			resyncDone:             closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - token seed not found",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			tokenSvcGetSeedErr: boom.Error,
			resyncDone:         closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - token seed not public",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			tokenSvcGetSeed: &provisioning.TokenSeed{Name: "default", Public: false},
			resyncDone:      closedChannel(),

			assertErr: errassert.Contains("must be public"),
		},
		{
			name:    "error - Operations Center address not configured",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "", // not configured
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              closedChannel(),

			assertErr: errassert.Contains("Operations Center address is not configured"),
		},
		{
			name:    "error - repo.GetByName",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameErr:        boom.Error,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:              closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - no BMC server client registered for type",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			tokenSvcGetSeed: &provisioning.TokenSeed{Name: "default", Public: true},
			resyncDone:      closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:    "error - client.AttachMedia",
			nameArg: "one",
			mediaArg: api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				Type:           "iso",
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			},
			operationsCenterAddress: "https://192.168.1.200:8443",
			repoGetByNameServer:     server,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Name: "default", Public: true},
			bmcClientAttachMediaErr: boom.Error,
			resyncDone:              closedChannel(),

			wantMediaURL:       "https://192.168.1.200:8443/1.0/provisioning/tokens/" + tokenUUID + "/seeds/default/architecture/x86_64/type/iso/" + testSeedImageID + ".iso",
			wantVirtualMediaID: "system:1",
			assertErr:          boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)

			if tc.operationsCenterAddress != "" {
				err := config.UpdateNetwork(t.Context(), system.NetworkPut{
					OperationsCenterAddress: tc.operationsCenterAddress,
					RestServerAddress:       "[::]:8443",
				})
				require.NoError(t, err)
			}

			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					return tc.repoUpdateErr
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					require.Equal(t, tc.mediaArg.Seed, name)

					return tc.tokenSvcGetSeed, tc.tokenSvcGetSeedErr
				},
				ResolveTokenSeedImageIDFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
					return testSeedImageID, tc.tokenSvcResolveImageIDErr
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					require.Equal(t, tc.mediaArg.Channel, name)

					return &provisioning.Channel{Name: name}, tc.channelSvcGetByNameErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				AttachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
					require.Equal(t, tc.wantVirtualMediaID, virtualMediaID)
					require.Equal(t, tc.wantMediaURL, mediaURL)
					require.Equal(t, tc.mediaArg.SetBootDevice, setBootDevice)

					return taskMonitor, tc.bmcClientAttachMediaErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return api.BMCData{}, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, tokenSvc, nil, channelSvc, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCAttachMediaByName(t.Context(), tc.nameArg, tc.mediaArg)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}

func TestServerService_BMCVirtualMediaSignal(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	const tokenUUID = "e9de436e-b94e-4aef-8563-883aec84096e"

	server := &provisioning.Server{
		Name: "one",
		BMCConfig: api.BMCConfig{
			APIType: api.BMCAPITypeRedfishV1Generic,
		},
	}

	setup := func(t *testing.T) (provisioning.ServerService, chan lifecycle.BMCVirtualMediaMessage, chan struct{}, *adapterMock.BMCServerClientPortMock) {
		t.Helper()

		config.InitTest(t, &envMock.EnvironmentMock{
			IsIncusOSFunc: func() bool {
				return false
			},
		}, nil)

		err := config.UpdateNetwork(t.Context(), system.NetworkPut{
			OperationsCenterAddress: "https://192.168.1.200:8443",
			RestServerAddress:       "[::]:8443",
		})
		require.NoError(t, err)

		resyncDone := make(chan struct{})

		repo := &repoMock.ServerRepoMock{
			GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
				return server, nil
			},
			UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
				defer close(resyncDone)

				return nil
			},
		}

		tokenSvc := &svcMock.TokenServiceMock{
			GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
				return &provisioning.TokenSeed{Name: name, Public: true}, nil
			},
			ResolveTokenSeedImageIDFunc: func(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (string, error) {
				return testSeedImageID, nil
			},
		}

		channelSvc := &svcMock.ChannelServiceMock{
			GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
				return &provisioning.Channel{Name: name}, nil
			},
		}

		bmcClient := &adapterMock.BMCServerClientPortMock{
			AttachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
				return nil, nil
			},
			DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
				return nil, nil
			},
			WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
				return nil
			},
			GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
				return api.BMCData{}, nil
			},
		}

		serverSvc := provisioningServer.New(
			repo, nil, nil, tokenSvc, nil, channelSvc, nil, tls.Certificate{},
			provisioningServer.WithNow(func() time.Time { return fixedDate }),
			provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
		)

		messages := make(chan lifecycle.BMCVirtualMediaMessage, 8)

		lifecycle.BMCVirtualMediaSignal.AddListener(func(ctx context.Context, msg lifecycle.BMCVirtualMediaMessage) {
			messages <- msg
		})

		t.Cleanup(lifecycle.BMCVirtualMediaSignal.Reset)

		t.Cleanup(serverSvc.WaitBackgroundTasks)

		return serverSvc, messages, resyncDone, bmcClient
	}

	awaitMessage := func(t *testing.T, messages chan lifecycle.BMCVirtualMediaMessage) lifecycle.BMCVirtualMediaMessage {
		t.Helper()

		select {
		case msg := <-messages:
			return msg

		case <-time.After(time.Second):
			t.Fatal("timed out waiting for the virtual media signal")
		}

		return lifecycle.BMCVirtualMediaMessage{}
	}

	awaitResync := func(t *testing.T, resyncDone chan struct{}) {
		t.Helper()

		select {
		case <-resyncDone:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for asynchronous BMC resync")
		}
	}

	t.Run("attach reports everything needed to prepare the image", func(t *testing.T) {
		serverSvc, messages, resyncDone, _ := setup(t)

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "iso",
			Architecture:   "x86_64",
			Channel:        "stable",
			VirtualMediaID: "system:1",
		})
		require.NoError(t, err)

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationPreAttach,
			Server:         "one",
			VirtualMediaID: "system:1",
			TokenUUID:      uuid.MustParse(tokenUUID),
			Seed:           "default",
			ImageType:      api.ImageTypeISO,
			Architecture:   images.UpdateFileArchitecture64BitX86,
			Channel:        "stable",
		}, awaitMessage(t, messages))

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationAttach,
			Server:         "one",
			VirtualMediaID: "system:1",
			TokenUUID:      uuid.MustParse(tokenUUID),
			Seed:           "default",
			ImageType:      api.ImageTypeISO,
			Architecture:   images.UpdateFileArchitecture64BitX86,
			Channel:        "stable",
		}, awaitMessage(t, messages))

		awaitResync(t, resyncDone)
	})

	t.Run("detach reports the virtual media it has been detached from", func(t *testing.T) {
		serverSvc, messages, resyncDone, _ := setup(t)

		err := serverSvc.BMCDetachMediaByName(t.Context(), "one", "system:1")
		require.NoError(t, err)

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationDetach,
			Server:         "one",
			VirtualMediaID: "system:1",
		}, awaitMessage(t, messages))

		awaitResync(t, resyncDone)
	})

	t.Run("the preparation is reported before the BMC is instructed", func(t *testing.T) {
		serverSvc, messages, resyncDone, bmcClient := setup(t)

		var reportedBeforeAttach []lifecycle.BMCVirtualMediaMessage

		bmcClient.AttachMediaFunc = func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
			for len(messages) > 0 {
				reportedBeforeAttach = append(reportedBeforeAttach, <-messages)
			}

			return nil, nil
		}

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "iso",
			Architecture:   "x86_64",
			Channel:        "stable",
			VirtualMediaID: "system:1",
		})
		require.NoError(t, err)

		require.Len(t, reportedBeforeAttach, 1, "the installation media has to be reported before the BMC is instructed to attach it")
		require.Equal(t, lifecycle.BMCVirtualMediaOperationPreAttach, reportedBeforeAttach[0].Operation)

		awaitResync(t, resyncDone)
	})

	t.Run("a failed attach only reports the preparation", func(t *testing.T) {
		serverSvc, messages, _, bmcClient := setup(t)

		bmcClient.AttachMediaFunc = func(ctx context.Context, server provisioning.Server, virtualMediaID string, mediaURL string, setBootDevice bool) (*provisioning.BMCTaskMonitor, error) {
			return nil, boom.Error
		}

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "iso",
			Architecture:   "x86_64",
			Channel:        "stable",
			VirtualMediaID: "system:1",
		})
		boom.ErrorIs(t, err)

		require.Equal(t, lifecycle.BMCVirtualMediaMessage{
			Operation:      lifecycle.BMCVirtualMediaOperationPreAttach,
			Server:         "one",
			VirtualMediaID: "system:1",
			TokenUUID:      uuid.MustParse(tokenUUID),
			Seed:           "default",
			ImageType:      api.ImageTypeISO,
			Architecture:   images.UpdateFileArchitecture64BitX86,
			Channel:        "stable",
		}, awaitMessage(t, messages))

		require.Empty(t, messages)
	})

	t.Run("a rejected attach request reports nothing", func(t *testing.T) {
		serverSvc, messages, _, _ := setup(t)

		err := serverSvc.BMCAttachMediaByName(t.Context(), "one", api.ServerBMCAttachMedia{
			TokenUUID:      tokenUUID,
			Seed:           "default",
			Type:           "exe",
			Architecture:   "x86_64",
			VirtualMediaID: "system:1",
		})
		require.Error(t, err)
		require.Empty(t, messages)
	})
}

func TestServerService_BMCDetachMediaByName(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	closedChannel := func() chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	tests := []struct {
		name                    string
		nameArg                 string
		virtualMediaIDArg       string
		repoGetByNameServer     *provisioning.Server
		repoGetByNameErr        error
		bmcClientDetachMediaErr error
		bmcClientWaitErr        error
		bmcClientGetDataErr     error
		repoUpdateErr           error
		resyncDone              chan struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:              "success",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			resyncDone: make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "success - task monitor wait fails but resync still runs",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientWaitErr: boom.Error,
			resyncDone:       make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "success - resync fails to get BMC data",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientGetDataErr: boom.Error,
			resyncDone:          make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "success - resync fails to update the server",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			repoUpdateErr: boom.Error,
			resyncDone:    make(chan struct{}),

			assertErr: require.NoError,
		},
		{
			name:              "error - name empty",
			nameArg:           "", // invalid
			virtualMediaIDArg: "system:1",
			resyncDone:        closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:              "error - virtual media ID empty",
			nameArg:           "one",
			virtualMediaIDArg: "", // invalid
			resyncDone:        closedChannel(),

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:              "error - repo.GetByName",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameErr:  boom.Error,
			resyncDone:        closedChannel(),

			assertErr: boom.ErrorIs,
		},
		{
			name:              "error - no BMC server client registered for type",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPIType("unknown"),
				},
			},
			resyncDone: closedChannel(),

			assertErr: errassert.Contains(`Failed to get BMC server client for type "unknown"`),
		},
		{
			name:              "error - client.DetachMedia",
			nameArg:           "one",
			virtualMediaIDArg: "system:1",
			repoGetByNameServer: &provisioning.Server{
				Name: "one",
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},
			bmcClientDetachMediaErr: boom.Error,
			resyncDone:              closedChannel(),

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.repoGetByNameServer, tc.repoGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					defer close(tc.resyncDone)

					return tc.repoUpdateErr
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
					require.Equal(t, tc.virtualMediaIDArg, virtualMediaID)

					return taskMonitor, tc.bmcClientDetachMediaErr
				},
				WaitForTaskFunc: func(ctx context.Context, server provisioning.Server, monitor *provisioning.BMCTaskMonitor) error {
					require.Same(t, taskMonitor, monitor)

					return tc.bmcClientWaitErr
				},
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					if tc.bmcClientGetDataErr != nil {
						close(tc.resyncDone)
					}

					return api.BMCData{}, tc.bmcClientGetDataErr
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return fixedDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			)

			// Run test
			err := serverSvc.BMCDetachMediaByName(t.Context(), tc.nameArg, tc.virtualMediaIDArg)

			serverSvc.WaitBackgroundTasks()

			// Assert
			tc.assertErr(t, err)

			select {
			case <-tc.resyncDone:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for asynchronous BMC resync")
			}
		})
	}
}
