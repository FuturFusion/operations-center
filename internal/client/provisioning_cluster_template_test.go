package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetClusterTemplates(t *testing.T) {
	socketClient, unauthorizedHTTPClient, db := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.ClusterTemplate)
	}{
		{
			name:       "success - empty list",
			client:     socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.ClusterTemplate) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:   "success - one record",
			client: socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateClusterTemplate(t.Context(), db, provisioning.ClusterTemplate{
					Name: "foo",
				})
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.ClusterTemplate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "foo", result[0].Name)
			},
		},
		{
			name:       "error - not authorized",
			client:     unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.ClusterTemplate) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetClusterTemplates(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetClusterTemplate(t *testing.T) {
	socketClient, unauthorizedHTTPClient, db := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.ClusterTemplate)
	}{
		{
			name:   "success - one record",
			client: socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateClusterTemplate(t.Context(), db, provisioning.ClusterTemplate{
					Name: "foo",
				})
				require.NoError(t, err)
			},

			tcNameArg: "foo",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.ClusterTemplate) {
				t.Helper()

				require.Equal(t, "foo", result.Name)
			},
		},
		{
			name:       "error - not authorized",
			client:     unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "foo",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.ClusterTemplate) {
				t.Helper()
			},
		},
		{
			name:       "error - not found",
			client:     socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",

			assertErr: func(t require.TestingT, err error, a ...any) {
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.ClusterTemplate) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetClusterTemplate(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_CreateClusterTemplate(t *testing.T) {
	socketClient, unauthorizedHTTPClient, db := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		clusterTemplate api.ClusterTemplatePost

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "success",
			client:     socketClient,
			dbSeedFunc: noop,

			clusterTemplate: api.ClusterTemplatePost{
				Name: "new-cluster-template",
				ClusterTemplatePut: api.ClusterTemplatePut{
					Description: "description",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:       "error - not authorized",
			client:     unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
		},
		{
			name:       "error - validation",
			client:     socketClient,
			dbSeedFunc: noop,

			clusterTemplate: api.ClusterTemplatePost{
				Name: "", // invalid no name provided
				ClusterTemplatePut: api.ClusterTemplatePut{
					Description: "description",
				},
			},

			assertErr: require.Error,
		},
		{
			name:   "error - confilict",
			client: socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateClusterTemplate(t.Context(), db, provisioning.ClusterTemplate{
					Name: "foo",
				})
				require.NoError(t, err)
			},

			clusterTemplate: api.ClusterTemplatePost{
				Name: "foo", // already exists
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CreateClusterTemplate(t.Context(), tc.clusterTemplate)

			tc.assertErr(t, err)
		})
	}
}

func Test_DeleteClusterTemplate(t *testing.T) {
	socketClient, unauthorizedHTTPClient, db := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:   "success - one record",
			client: socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateClusterTemplate(t.Context(), db, provisioning.ClusterTemplate{
					Name: "foo",
				})
				require.NoError(t, err)
			},

			tcNameArg: "foo",

			assertErr: require.NoError,
		},
		{
			name:       "error - not authorized",
			client:     unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: "foo",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
		},
		{
			name:       "error - not found",
			client:     socketClient,
			dbSeedFunc: noop,

			tcNameArg: "unknown",

			assertErr: func(t require.TestingT, err error, a ...any) {
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteClusterTemplate(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
		})
	}
}
