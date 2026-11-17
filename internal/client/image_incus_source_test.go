package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	imageEntities "github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetImageIncusSources(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.ImageSource)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.ImageSource) {
				t.Helper()

				require.Len(t, result, 1, "a fresh database has the local source")
				require.Equal(t, "local", result[0].Name)
			},
		},
		{
			name:   "success - one record",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedImageIncusSource(t, d)
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.ImageSource) {
				t.Helper()

				require.Len(t, result, 2, "the local source and the added source")
				require.Equal(t, "source_one", result[1].Name)
				require.Equal(t, "https://images.example.com", result[1].URL)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.ImageSource) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetImageIncusSources(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetImageIncusSource(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.ImageSource)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedImageIncusSource(t, d)
			},

			tcNameArg: "source_one",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.ImageSource) {
				t.Helper()

				require.Equal(t, "source_one", result.Name)
				require.Equal(t, "https://images.example.com", result.URL)
				require.Equal(t, `architecture == "amd64"`, result.FilterExpression)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "source_one",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.ImageSource) {
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
			assertFunc: func(t *testing.T, result api.ImageSource) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetImageIncusSource(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_CreateImageIncusSource(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		imageSource api.ImageSourcePost

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:       "success",
			client:     d.socketClient,
			dbSeedFunc: noop,

			imageSource: api.ImageSourcePost{
				Name: "new_source",
				ImageSourcePut: api.ImageSourcePut{
					URL:              "https://images.example.com",
					FilterExpression: `architecture == "amd64"`,
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				imageSource, err := d.socketClient.GetImageIncusSource(t.Context(), "new_source")
				require.NoError(t, err)
				require.Equal(t, "https://images.example.com", imageSource.URL)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			imageSource: api.ImageSourcePost{
				Name: "unauthorized",
				ImageSourcePut: api.ImageSourcePut{
					FilterExpression: "true",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, empty name",
			client:     d.socketClient,
			dbSeedFunc: noop,

			imageSource: api.ImageSourcePost{
				Name: "",
				ImageSourcePut: api.ImageSourcePut{
					FilterExpression: "true",
				},
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
		{
			name:       "error - validation, empty filter expression",
			client:     d.socketClient,
			dbSeedFunc: noop,

			imageSource: api.ImageSourcePost{
				Name: "without_filter_expression",
				ImageSourcePut: api.ImageSourcePut{
					URL: "https://images.example.com",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Empty filter expression is not permitted")
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, invalid filter expression",
			client:     d.socketClient,
			dbSeedFunc: noop,

			imageSource: api.ImageSourcePost{
				Name: "invalid_filter_expression",
				ImageSourcePut: api.ImageSourcePut{
					URL:              "https://images.example.com",
					FilterExpression: "this is not a valid expression",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "failed to validate filter expression")
			},
			assertFunc: noop,
		},
		{
			name:       "error - conflict",
			client:     d.socketClient,
			dbSeedFunc: noop,

			imageSource: api.ImageSourcePost{
				Name: "new_source", // already exists
				ImageSourcePut: api.ImageSourcePut{
					FilterExpression: "true",
				},
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CreateImageIncusSource(t.Context(), tc.imageSource)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_UpdateImageIncusSource(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg   string
		imageSource api.ImageSourcePut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedImageIncusSource(t, d)
			},

			tcNameArg: "source_one",
			imageSource: api.ImageSourcePut{
				URL:              "https://updated.example.com",
				FilterExpression: `architecture == "arm64"`,
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				imageSource, err := d.socketClient.GetImageIncusSource(t.Context(), "source_one")
				require.NoError(t, err)
				require.Equal(t, "https://updated.example.com", imageSource.URL)
				require.Equal(t, `architecture == "arm64"`, imageSource.FilterExpression)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "source_one",

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
			imageSource: api.ImageSourcePut{
				FilterExpression: "true",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, empty filter expression",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "source_one",
			imageSource: api.ImageSourcePut{
				URL: "https://images.example.com",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Empty filter expression is not permitted")
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateImageIncusSource(t.Context(), tc.tcNameArg, tc.imageSource)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteImageIncusSource(t *testing.T) {
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

				seedImageIncusSource(t, d)
			},

			tcNameArg: "source_one",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetImageIncusSource(t.Context(), "source_one")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "source_one",

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

			err := tc.client.DeleteImageIncusSource(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

// Test_RefreshImageIncusSource only covers the error paths, which are handled
// before the remote simplestreams server is contacted.
func Test_RefreshImageIncusSource(t *testing.T) {
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

			tcNameArg: "source_one",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
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
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.RefreshImageIncusSource(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
		})
	}
}

func seedImageIncusSource(t *testing.T, d testDaemon) {
	t.Helper()

	_, err := imageEntities.CreateIncusImageSource(t.Context(), d.db, image.IncusImageSource{
		Name:             "source_one",
		URL:              "https://images.example.com",
		FilterExpression: `architecture == "amd64"`,
	})
	require.NoError(t, err)
}
