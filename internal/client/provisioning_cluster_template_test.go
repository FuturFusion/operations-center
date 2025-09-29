package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetClusterTemplates(t *testing.T) {
	httpClient, _, db := daemonSetup(t)

	tests := []struct {
		name       string
		dbSeedFunc func(t *testing.T)

		assertFunc func(t *testing.T, result []api.ClusterTemplate)
	}{
		{
			name:       "empty",
			dbSeedFunc: noop,

			assertFunc: func(t *testing.T, result []api.ClusterTemplate) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name: "one record",

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateClusterTemplate(t.Context(), db, provisioning.ClusterTemplate{
					Name: "foo",
				})
				require.NoError(t, err)
			},

			assertFunc: func(t *testing.T, result []api.ClusterTemplate) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "foo", result[0].Name)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := httpClient.GetClusterTemplates(t.Context())
			require.NoError(t, err)

			tc.assertFunc(t, result)
		})
	}
}
