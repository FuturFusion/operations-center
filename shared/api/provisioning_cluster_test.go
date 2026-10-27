package api_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClusterFilter_URLValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter api.ClusterFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: api.ClusterFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: api.ClusterFilter{
				Expression: new("true"),
			},

			want: `filter=true`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			query := tc.filter.AppendToURLValues(url.Values{})
			filter := api.ClusterFilterFromURLValues(query)

			require.Equal(t, tc.want, query.Encode(), "the filter is encoded as query parameters")
			require.Equal(t, tc.filter, filter, "parsing the query returns the original filter")
		})
	}
}
