package client_test

import (
	"testing"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

const defaultChannelName = "stable"

func Test_GetChannels(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Channel)
	}{
		{
			name:       "success - only the default channel",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Channel) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, defaultChannelName, result[0].Name)
			},
		},
		{
			name:   "success - two records",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateChannel(t.Context(), d.db, provisioning.Channel{
					Name:        "testing",
					Description: "Testing updates channel",
				})
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Channel) {
				t.Helper()

				require.Len(t, result, 2)

				names := make([]string, 0, len(result))
				for _, channel := range result {
					names = append(names, channel.Name)
				}

				require.ElementsMatch(t, []string{defaultChannelName, "testing"}, names)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.Channel) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetChannels(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetChannel(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.Channel)
	}{
		{
			name:       "success - default channel",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: defaultChannelName,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Channel) {
				t.Helper()

				require.Equal(t, defaultChannelName, result.Name)
			},
		},
		{
			name:   "success - seeded channel",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateChannel(t.Context(), d.db, provisioning.Channel{
					Name:        "testing",
					Description: "Testing updates channel",
				})
				require.NoError(t, err)
			},

			tcNameArg: "testing",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Channel) {
				t.Helper()

				require.Equal(t, "testing", result.Name)
				require.Equal(t, "Testing updates channel", result.Description)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: defaultChannelName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.Channel) {
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
			assertFunc: func(t *testing.T, result api.Channel) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetChannel(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_CreateChannel(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		channel api.ChannelPost

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:       "success",
			client:     d.socketClient,
			dbSeedFunc: noop,

			channel: api.ChannelPost{
				Name: "testing",
				ChannelPut: api.ChannelPut{
					Description: "Testing updates channel",
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				channel, err := d.socketClient.GetChannel(t.Context(), "testing")
				require.NoError(t, err)
				require.Equal(t, "Testing updates channel", channel.Description)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			channel: api.ChannelPost{
				Name: "unauthorized",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation",
			client:     d.socketClient,
			dbSeedFunc: noop,

			channel: api.ChannelPost{
				Name: "", // invalid, no name provided
				ChannelPut: api.ChannelPut{
					Description: "Channel without a name",
				},
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
		{
			name:       "error - conflict",
			client:     d.socketClient,
			dbSeedFunc: noop,

			channel: api.ChannelPost{
				Name: defaultChannelName, // already exists
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CreateChannel(t.Context(), tc.channel)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_UpdateChannel(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		channel   api.ChannelPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateChannel(t.Context(), d.db, provisioning.Channel{
					Name:        "testing",
					Description: "Testing updates channel",
				})
				require.NoError(t, err)
			},

			tcNameArg: "testing",
			channel: api.ChannelPut{
				Description: "Updated description",
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				channel, err := d.socketClient.GetChannel(t.Context(), "testing")
				require.NoError(t, err)
				require.Equal(t, "Updated description", channel.Description)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "testing",

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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateChannel(t.Context(), tc.tcNameArg, tc.channel)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteChannel(t *testing.T) {
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

				_, err := entities.CreateChannel(t.Context(), d.db, provisioning.Channel{
					Name: "testing",
				})
				require.NoError(t, err)
			},

			tcNameArg: "testing",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetChannel(t.Context(), "testing")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: defaultChannelName,

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
			name:   "error - channel is in use by an update",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateChannel(t.Context(), d.db, provisioning.Channel{
					Name: "in-use",
				})
				require.NoError(t, err)

				_, err = entities.CreateUpdate(t.Context(), d.db, provisioning.Update{
					UUID:     uuidgen.FromPattern(t, "1"),
					Origin:   "test",
					Version:  "1",
					Severity: images.UpdateSeverityNone,
					Status:   api.UpdateStatusReady,
				})
				require.NoError(t, err)

				// Assign the update to the channel through the API.
				err = d.socketClient.UpdateUpdate(t.Context(), uuidgen.FromPattern(t, "1").String(), api.UpdatePut{
					Channels: []string{"in-use"},
				})
				require.NoError(t, err)
			},

			tcNameArg: "in-use",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Channel "in-use" is assigned to 1 updates and can not be deleted`)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteChannel(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_GetChannelChangelog(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: defaultChannelName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
		},
		{
			name:       "error - channel without updates",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: defaultChannelName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "does not contain any updates")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			_, err := tc.client.GetChannelChangelog(t.Context(), tc.tcNameArg, "x86_64")

			tc.assertErr(t, err)
		})
	}
}
