package client_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetServers(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Server)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:   "success - one record",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedServer(t, d, "serverOne", nil)
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "serverOne", result[0].Name)
				require.Equal(t, defaultChannelName, result[0].Channel)
				require.Equal(t, api.ServerStatusUnregistered, result[0].Status)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetServers(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetWithFilterServers(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		filter provisioning.ServerFilter

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Server)
	}{
		{
			name:   "success - filter by cluster, matching",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateCluster(t.Context(), d.db, provisioning.Cluster{
					Name:    "clusterOne",
					Channel: defaultChannelName,
				})
				require.NoError(t, err)

				seedServer(t, d, "clusteredServer", new("clusterOne"))
				seedServer(t, d, "standaloneServer", nil)
			},

			filter: provisioning.ServerFilter{
				Cluster: new("clusterOne"),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "clusteredServer", result[0].Name)
			},
		},
		{
			name:       "success - filter by cluster, not matching",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.ServerFilter{
				Cluster: new("unknown"),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:       "success - filter by status",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.ServerFilter{
				Status: new(api.ServerStatusUnregistered),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()

				require.Len(t, result, 2)
			},
		},
		{
			name:       "success - filter by expression",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.ServerFilter{
				Expression: new(`name == "standaloneServer"`),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "standaloneServer", result[0].Name)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.Server) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetWithFilterServers(t.Context(), tc.filter)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetServer(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.Server)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedServer(t, d, "serverOne", nil)
			},

			tcNameArg: "serverOne",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Server) {
				t.Helper()

				require.Equal(t, "serverOne", result.Name)
				require.Equal(t, "Test server serverOne", result.Description)
				require.Empty(t, result.Cluster)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "serverOne",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.Server) {
				t.Helper()
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.Server) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetServer(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_PreRegisterServer(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		server api.ServerPost

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:       "success",
			client:     d.socketClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: "newServer",
				ServerPut: api.ServerPut{
					Channel:     defaultChannelName,
					Description: "pre registered server",
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				server, err := d.socketClient.GetServer(t.Context(), "newServer")
				require.NoError(t, err)
				require.Equal(t, "pre registered server", server.Description)
				require.Equal(t, api.ServerStatusUnregistered, server.Status)
			},
		},
		{
			// POST /1.0/provisioning/servers is exempt from authentication,
			// because it can be authenticated with a registration token instead.
			// Without a token, the request is rejected by the authorization
			// check performed by the handler.
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: "unauthorizedServer",
				ServerPut: api.ServerPut{
					Channel: defaultChannelName,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthorized)
			},
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetServer(t.Context(), "unauthorizedServer")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "success - authorized with TLS client certificate",
			client:     d.authorizedHTTPClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: "tlsServer",
				ServerPut: api.ServerPut{
					Channel: defaultChannelName,
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetServer(t.Context(), "tlsServer")
				require.NoError(t, err)
			},
		},
		{
			name:       "error - validation, empty name",
			client:     d.socketClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: "",
				ServerPut: api.ServerPut{
					Channel: defaultChannelName,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "name can not be empty")
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, empty channel",
			client:     d.socketClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: "withoutChannel",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "channel can not be empty")
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, reserved name prefix",
			client:     d.socketClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: ":reserved",
				ServerPut: api.ServerPut{
					Channel: defaultChannelName,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "is reserved for internal use")
			},
			assertFunc: noop,
		},
		{
			name:       "error - conflict",
			client:     d.socketClient,
			dbSeedFunc: noop,

			server: api.ServerPost{
				Name: "newServer", // already exists
				ServerPut: api.ServerPut{
					Channel: defaultChannelName,
				},
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.PreRegisterServer(t.Context(), tc.server)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_UpdateServer(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		server    api.ServerPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			// The channel is left unchanged, changing it triggers an update of
			// the system update configuration on the server itself.
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedServer(t, d, "serverOne", nil)
			},

			tcNameArg: "serverOne",
			server: api.ServerPut{
				Channel:             defaultChannelName,
				Description:         "updated description",
				PublicConnectionURL: "https://public.example.com:6443",
				Properties: api.ConfigMap{
					"rack": "A1",
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				server, err := d.socketClient.GetServer(t.Context(), "serverOne")
				require.NoError(t, err)
				require.Equal(t, "updated description", server.Description)
				require.Equal(t, "https://public.example.com:6443", server.PublicConnectionURL)
				require.Equal(t, api.ConfigMap{"rack": "A1"}, server.Properties)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "serverOne",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",
			server: api.ServerPut{
				Channel: defaultChannelName,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, empty channel",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "serverOne",
			server:    api.ServerPut{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "channel can not be empty")
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, BMC endpoint missing",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "serverOne",
			server: api.ServerPut{
				Channel: defaultChannelName,
				BMCConfig: api.BMCConfig{
					APIType: api.BMCAPITypeRedfishV1Generic,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "BMC endpoint can not be empty")
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateServer(t.Context(), tc.tcNameArg, tc.server)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_RenameServer(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		tcNewName string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedServer(t, d, "serverOne", nil)
			},

			tcNameArg: "serverOne",
			tcNewName: "serverRenamed",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				server, err := d.socketClient.GetServer(t.Context(), "serverRenamed")
				require.NoError(t, err)
				require.Equal(t, "serverRenamed", server.Name)

				_, err = d.socketClient.GetServer(t.Context(), "serverOne")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "serverRenamed",
			tcNewName: "other",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",
			tcNewName: "other",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - new name is empty",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "serverRenamed",
			tcNewName: "",

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.RenameServer(t.Context(), tc.tcNameArg, tc.tcNewName)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteServer(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedServer(t, d, "serverOne", nil)
			},

			tcNameArg: "serverOne",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetServer(t.Context(), "serverOne")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "serverOne",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:   "error - server is part of a cluster",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateCluster(t.Context(), d.db, provisioning.Cluster{
					Name:    "clusterOne",
					Channel: defaultChannelName,
				})
				require.NoError(t, err)

				seedServer(t, d, "clusteredServer", new("clusterOne"))
			},

			tcNameArg: "clusteredServer",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `is a member of cluster "clusterOne" and can not be deleted`)
			},
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetServer(t.Context(), "clusteredServer")
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteServer(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func seedServer(t *testing.T, d testDaemon, name string, cluster *string) {
	t.Helper()

	_, err := entities.CreateServer(t.Context(), d.db, provisioning.Server{
		Name:         name,
		Cluster:      cluster,
		Channel:      defaultChannelName,
		Status:       api.ServerStatusUnregistered,
		StatusDetail: api.ServerStatusDetailNone,
		Description:  "Test server " + name,
	})
	require.NoError(t, err)
}

// serverActionRoutes are the routes below /1.0/provisioning/servers/{name},
// which require a 3rd party system (BMC/Redfish or the IncusOS instance of the
// server itself) to be reachable. Only the error paths, which are handled
// before the 3rd party system is contacted, are covered.
func serverActionRoutes(t *testing.T) []struct {
	name string
	call func(ctx context.Context, c client.OperationsCenterClient, serverName string) error
} {
	t.Helper()

	tokenUUID := uuidgen.FromPattern(t, "1").String()

	return []struct {
		name string
		call func(ctx context.Context, c client.OperationsCenterClient, serverName string) error
	}{
		{"ResyncServer", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.ResyncServer(ctx, n)
		}},
		{"GetServerChangelog", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerChangelog(ctx, n)
			return err
		}},
		{"EvacuateServerSystem", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.EvacuateServerSystem(ctx, n, false)
		}},
		{"FactoryResetServerSystem", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.FactoryResetServerSystem(ctx, n)
		}},
		{"PoweroffServerSystem", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.PoweroffServerSystem(ctx, n, false)
		}},
		{"RebootServerSystem", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.RebootServerSystem(ctx, n, false)
		}},
		{"RestoreServerSystem", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.RestoreServerSystem(ctx, n, false)
		}},
		{"UpdateServerSystem", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.UpdateServerSystem(ctx, n, api.ServerUpdatePost{}, false)
		}},
		{"GetServerSystemNetwork", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerSystemNetwork(ctx, n)
			return err
		}},
		{"UpdateServerSystemNetwork", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.UpdateServerSystemNetwork(ctx, n, api.ServerSystemNetwork{})
		}},
		{"GetServerSystemStorage", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerSystemStorage(ctx, n)
			return err
		}},
		{"UpdateServerSystemStorage", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.UpdateServerSystemStorage(ctx, n, api.ServerSystemStorage{})
		}},
		{"BMCDataRefresh", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCDataRefresh(ctx, n)
		}},
		{"BMCServerPowerOn", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCServerPowerOn(ctx, n, false)
		}},
		{"BMCServerPowerOff", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCServerPowerOff(ctx, n, false)
		}},
		{"BMCServerRestart", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCServerRestart(ctx, n, false)
		}},
		{"BMCServerSetLocationIndicator", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCServerSetLocationIndicator(ctx, n, true)
		}},
		{"GetServerBIOSProfile", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBIOSProfile(ctx, n, false)
			return err
		}},
		{"ApplyBIOSAttributes", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.ApplyBIOSAttributes(ctx, n, map[string]any{"attr": "value"})
		}},
		{"GetServerBMCBIOSAttributes", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCBIOSAttributes(ctx, n)
			return err
		}},
		{"GetServerBMCBIOSAttributeAcceptableValues", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCBIOSAttributeAcceptableValues(ctx, n, "attr")
			return err
		}},
		{"BMCApplySecureBootCertificates", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCApplySecureBootCertificates(ctx, n)
		}},
		{"GetServerBMCLogSources", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCLogSources(ctx, n)
			return err
		}},
		{"GetServerBMCLogEntries", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCLogEntries(ctx, n, "system/Sel")
			return err
		}},
		{"GetServerBMCDump", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCDump(ctx, n, nil, false, false)
			return err
		}},
		{"BMCAttachMedia", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCAttachMedia(ctx, n, api.ServerBMCAttachMedia{
				TokenUUID:      tokenUUID,
				Seed:           "some-seed",
				Type:           string(api.ImageTypeISO),
				Architecture:   "x86_64",
				VirtualMediaID: "system:1",
			})
		}},
		{"BMCDetachMedia", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.BMCDetachMedia(ctx, n, api.ServerBMCDetachMedia{
				VirtualMediaID: "system:1",
			})
		}},
		{"DeployServer", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.DeployServer(ctx, n, api.ServerDeploymentPost{
				TokenUUID:    tokenUUID,
				Seed:         "some-seed",
				Type:         string(api.ImageTypeISO),
				Architecture: "x86_64",
			})
		}},
		{"CancelServerDeployment", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.CancelServerDeployment(ctx, n, false)
		}},
	}
}

func Test_ServerActionRoutes_notAuthorized(t *testing.T) {
	d := daemonSetup(t)

	for _, tc := range serverActionRoutes(t) {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t.Context(), d.unauthorizedHTTPClient, "serverOne")

			require.ErrorIs(t, err, domain.ErrNotAuthenticated)
		})
	}
}

func Test_ServerActionRoutes_notFound(t *testing.T) {
	d := daemonSetup(t)

	for _, tc := range serverActionRoutes(t) {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t.Context(), d.socketClient, "unknown")

			require.ErrorIs(t, err, domain.ErrNotFound)
		})
	}
}
