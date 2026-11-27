package client_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	imageEntities "github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	// incusImageName has to match the format "os:release:architecture:variant".
	incusImageName    = "almalinux:10:amd64:default"
	incusImageVersion = "202601011200"
)

func Test_GetIncusImages(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.IncusImage)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.IncusImage) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:   "success - one record",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedIncusImage(t, d)
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.IncusImage) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, incusImageName, result[0].Name)
				require.Contains(t, result[0].Versions, incusImageVersion)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.IncusImage) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetIncusImages(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetIncusImage(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.IncusImage)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedIncusImage(t, d)
			},

			tcNameArg: incusImageName,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.IncusImage) {
				t.Helper()

				require.Equal(t, incusImageName, result.Name)
				require.Equal(t, "almalinux", result.OperatingSystem)
				require.Equal(t, "10", result.Release)
				require.Equal(t, "amd64", result.Architecture)
				require.Equal(t, "default", result.Variant)
				require.Equal(t, api.IncusImageAlias{"almalinux/10"}, result.Aliases)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: incusImageName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.IncusImage) {
				t.Helper()
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "almalinux:9:amd64:default",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.IncusImage) {
				t.Helper()
			},
		},
		{
			name:       "error - invalid image name",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "not-a-valid-image-name",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `expect name in the format "os:release:architecture:variant"`)
			},
			assertFunc: func(t *testing.T, result api.IncusImage) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetIncusImage(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_UpdateIncusImage(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg  string
		incusImage api.IncusImagePut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedIncusImage(t, d)
			},

			tcNameArg: incusImageName,
			incusImage: api.IncusImagePut{
				Aliases:     api.IncusImageAlias{"almalinux/10/amd64"},
				Description: "updated description",
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				incusImage, err := d.socketClient.GetIncusImage(t.Context(), incusImageName)
				require.NoError(t, err)
				require.Equal(t, "updated description", incusImage.Description)
				require.Equal(t, api.IncusImageAlias{"almalinux/10/amd64"}, incusImage.Aliases)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: incusImageName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "almalinux:9:amd64:default",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - invalid image name",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "not-a-valid-image-name",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `expect name in the format "os:release:architecture:variant"`)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateIncusImage(t.Context(), tc.tcNameArg, tc.incusImage)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_GetIncusImageVersionFile(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg     string
		tcVersionArg  string
		tcFilenameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result io.ReadCloser)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedIncusImage(t, d)
				seedFile(t, d, filepath.Join("images", "almalinux", "10", "amd64", "default", incusImageVersion, "incus.tar.xz"), []byte("image-metadata"))
			},

			tcNameArg:     incusImageName,
			tcVersionArg:  incusImageVersion,
			tcFilenameArg: "incus.tar.xz",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()

				defer result.Close()

				content, err := io.ReadAll(result)
				require.NoError(t, err)
				require.Equal(t, "image-metadata", string(content))
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:     incusImageName,
			tcVersionArg:  incusImageVersion,
			tcFilenameArg: "incus.tar.xz",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()
			},
		},
		{
			name:       "error - unknown image",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     "almalinux:9:amd64:default",
			tcVersionArg:  incusImageVersion,
			tcFilenameArg: "incus.tar.xz",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result io.ReadCloser) {
				t.Helper()
			},
		},
		{
			name:       "error - unknown file",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     incusImageName,
			tcVersionArg:  incusImageVersion,
			tcFilenameArg: "unknown.tar.xz",

			assertErr:  require.Error,
			assertFunc: func(t *testing.T, result io.ReadCloser) { t.Helper() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetIncusImageVersionFile(t.Context(), tc.tcNameArg, tc.tcVersionArg, tc.tcFilenameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_DeleteIncusImageVersion(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg    string
		tcVersionArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedIncusImage(t, d)
				seedFile(t, d, filepath.Join("images", "almalinux", "10", "amd64", "default", incusImageVersion, "incus.tar.xz"), []byte("image-metadata"))
			},

			tcNameArg:    incusImageName,
			tcVersionArg: incusImageVersion,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				incusImage, err := d.socketClient.GetIncusImage(t.Context(), incusImageName)
				require.NoError(t, err)
				require.Empty(t, incusImage.Versions)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:    incusImageName,
			tcVersionArg: incusImageVersion,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - unknown version",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:    incusImageName,
			tcVersionArg: "202512311200",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - unknown image",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:    "almalinux:9:amd64:default",
			tcVersionArg: incusImageVersion,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteIncusImageVersion(t.Context(), tc.tcNameArg, tc.tcVersionArg)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteIncusImage(t *testing.T) {
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

				seedIncusImage(t, d)
				seedFile(t, d, filepath.Join("images", "almalinux", "10", "amd64", "default", incusImageVersion, "incus.tar.xz"), []byte("image-metadata"))
			},

			tcNameArg: incusImageName,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetIncusImage(t.Context(), incusImageName)
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: incusImageName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: incusImageName,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteIncusImage(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

// Test_CreateIncusImageVersion only covers the not authorized case, uploading
// an image version requires a complete image archive.
func Test_CreateIncusImageVersion(t *testing.T) {
	d := daemonSetup(t)

	err := d.unauthorizedHTTPClient.CreateIncusImageVersion(t.Context(), nil)
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

func seedIncusImage(t *testing.T, d testDaemon) {
	t.Helper()

	_, err := imageEntities.CreateIncusImage(t.Context(), d.db, image.IncusImage{
		Name:            incusImageName,
		Aliases:         []string{"almalinux/10"},
		Description:     "AlmaLinux 10",
		OperatingSystem: "almalinux",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "default",
		MemberID:        1,
		Versions: api.IncusImageVersions{
			incusImageVersion: api.IncusImageVersion{
				Items: map[string]api.IncusImageVersionItem{
					"incus.tar.xz": {
						FileType: "incus.tar.xz",
						Path:     "images/almalinux/10/amd64/default/" + incusImageVersion + "/incus.tar.xz",
						Size:     14,
					},
				},
			},
		},
	})
	require.NoError(t, err)
}
