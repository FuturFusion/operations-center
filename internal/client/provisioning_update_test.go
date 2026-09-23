package client_test

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	updateVersion  = "1.0"
	updateFilename = "IncusOS_1.0.efi.gz"
)

// The GET routes below /1.0/provisioning/updates are exempt from
// authentication. Therefore requests performed with the unauthorized client are
// expected to succeed, while the mutating routes still require authentication.

func Test_GetUpdates(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Update)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:   "success - one record",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, uuidgen.FromPattern(t, "1"), result[0].UUID)
				require.Equal(t, updateVersion, result[0].Version)
				require.Equal(t, "test-origin", result[0].Origin)
			},
		},
		{
			name:       "success - authentication is not required",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()

				require.Len(t, result, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetUpdates(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetWithFilterUpdates(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		filter provisioning.UpdateFilter

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Update)
	}{
		{
			name:   "success - filter by origin, matching",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
			},

			filter: provisioning.UpdateFilter{
				Origin: new("test-origin"),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()

				require.Len(t, result, 1)
			},
		},
		{
			name:       "success - filter by origin, not matching",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.UpdateFilter{
				Origin: new("unknown-origin"),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:       "success - filter by status",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.UpdateFilter{
				Status: new(api.UpdateStatusReady),
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()

				require.Len(t, result, 1)
			},
		},
		{
			name:       "error - invalid status",
			client:     d.socketClient,
			dbSeedFunc: noop,

			filter: provisioning.UpdateFilter{
				Status: new(api.UpdateStatus("invalid")),
			},

			assertErr: require.Error,
			assertFunc: func(t *testing.T, result []api.Update) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetWithFilterUpdates(t.Context(), tc.filter)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetUpdate(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.Update)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Update) {
				t.Helper()

				require.Equal(t, uuidgen.FromPattern(t, "1"), result.UUID)
				require.Equal(t, updateVersion, result.Version)
				require.Equal(t, api.UpdateStatusReady, result.Status)
			},
		},
		{
			name:       "success - authentication is not required",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Update) {
				t.Helper()

				require.Equal(t, uuidgen.FromPattern(t, "1"), result.UUID)
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "2").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.Update) {
				t.Helper()
			},
		},
		{
			name:       "error - invalid uuid",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "not-a-uuid",

			assertErr: require.Error,
			assertFunc: func(t *testing.T, result api.Update) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetUpdate(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetUpdateFiles(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.UpdateFile)
	}{
		{
			name:   "success - one file",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.UpdateFile) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, updateFilename, result[0].Filename)
				require.Equal(t, images.UpdateFileArchitecture64BitX86, result[0].Architecture)
			},
		},
		{
			name:       "success - authentication is not required",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.UpdateFile) {
				t.Helper()

				require.Len(t, result, 1)
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "2").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result []api.UpdateFile) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetUpdateFiles(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetUpdatesFile(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg     string
		tcFilenameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result io.ReadCloser)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
				seedFile(t, d, filepath.Join("updates", uuidgen.FromPattern(t, "1").String(), updateFilename), []byte("update-file"))
			},

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcFilenameArg: updateFilename,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()

				defer result.Close()

				content, err := io.ReadAll(result)
				require.NoError(t, err)
				require.Equal(t, "update-file", string(content))
			},
		},
		{
			name:       "success - authentication is not required",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcFilenameArg: updateFilename,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()

				defer result.Close()

				content, err := io.ReadAll(result)
				require.NoError(t, err)
				require.Equal(t, "update-file", string(content))
			},
		},
		{
			name:       "error - file is not part of the update",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcFilenameArg: "unknown.efi.gz",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()
			},
		},
		{
			name:       "error - unknown update",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "2").String(),
			tcFilenameArg: updateFilename,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetUpdatesFile(t.Context(), tc.tcNameArg, tc.tcFilenameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_UpdateUpdate(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		update    api.UpdatePut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success - assign a channel",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			update: api.UpdatePut{
				Channels: []string{defaultChannelName},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				update, err := d.socketClient.GetUpdate(t.Context(), uuidgen.FromPattern(t, "1").String())
				require.NoError(t, err)
				require.Equal(t, []string{defaultChannelName}, update.Channels)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "success - authorized with TLS client certificate",
			client:     d.authorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			update: api.UpdatePut{
				Channels: []string{},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				update, err := d.socketClient.GetUpdate(t.Context(), uuidgen.FromPattern(t, "1").String())
				require.NoError(t, err)
				require.Empty(t, update.Channels)
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "2").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - unknown channel",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			update: api.UpdatePut{
				Channels: []string{"unknown"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
				require.ErrorContains(tt, err, "can not be assigned to the channels [unknown]")
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateUpdate(t.Context(), tc.tcNameArg, tc.update)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_CleanupAllUpdates(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedUpdate(t, d, uuidgen.FromPattern(t, "1"))
				seedFile(t, d, filepath.Join("updates", uuidgen.FromPattern(t, "1").String(), updateFilename), []byte("update-file"))
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				updates, err := d.socketClient.GetUpdates(t.Context())
				require.NoError(t, err)
				require.Empty(t, updates)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CleanupAllUpdates(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

// Test_GetUpdateChangelog only covers the error paths, computing a changelog
// requires the manifest files of the update to be present.
func Test_GetUpdateChangelog(t *testing.T) {
	d := daemonSetup(t)

	_, err := d.socketClient.GetUpdateChangelog(t.Context(), uuidgen.FromPattern(t, "2").String(), defaultChannelName, false, "x86_64")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

// Test_RefreshUpdates only covers the not authorized case, a refresh contacts
// the remote update source.
func Test_RefreshUpdates(t *testing.T) {
	d := daemonSetup(t)

	err := d.unauthorizedHTTPClient.RefreshUpdates(t.Context(), true)
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

func seedUpdate(t *testing.T, d testDaemon, id uuid.UUID) {
	t.Helper()

	_, err := entities.CreateUpdate(t.Context(), d.db, provisioning.Update{
		UUID:        id,
		Origin:      "test-origin",
		Version:     updateVersion,
		PublishedAt: time.Now().UTC().Truncate(time.Second),
		Severity:    images.UpdateSeverityNone,
		Status:      api.UpdateStatusReady,
		Files: provisioning.UpdateFiles{
			{
				Filename:     updateFilename,
				Size:         11,
				Component:    images.UpdateFileComponentOS,
				Type:         images.UpdateFileTypeUpdateEFI,
				Architecture: images.UpdateFileArchitecture64BitX86,
			},
		},
	})
	require.NoError(t, err)
}
