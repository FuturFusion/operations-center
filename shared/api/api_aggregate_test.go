package api_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestInventoryAggregateFilter_URLValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter api.InventoryAggregateFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: api.InventoryAggregateFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: api.InventoryAggregateFilter{
				Kinds:              []string{"kind"},
				Clusters:           []string{"cluster"},
				Servers:            []string{"server"},
				ServerIncludeNull:  true,
				Projects:           []string{"project"},
				ProjectIncludeNull: true,
				Parents:            []string{"parent"},
				ParentIncludeNull:  true,
				Expression:         new("true"),
			},

			want: `cluster=cluster&filter=true&kind=kind&parent=parent&parent_include_null=true&project=project&project_include_null=true&server=server&server_include_null=true`,
		},
		{
			name: "multiple values",
			filter: api.InventoryAggregateFilter{
				Kinds:    []string{"instances", "images"},
				Clusters: []string{"one", "two"},
			},

			want: `cluster=one&cluster=two&kind=instances&kind=images`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			query := tc.filter.AppendToURLValues(url.Values{})
			filter, err := api.InventoryAggregateFilterFromURLValues(query)

			require.NoError(t, err)
			require.Equal(t, tc.want, query.Encode(), "the filter is encoded as query parameters")
			require.Equal(t, tc.want, tc.filter.String(), "String returns the encoded query")
			require.Equal(t, tc.filter, filter, "parsing the query returns the original filter")
		})
	}
}

func TestInventoryAggregateFilterFromURLValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query url.Values

		assertErr require.ErrorAssertionFunc
		want      api.InventoryAggregateFilter
	}{
		{
			name:  "empty query",
			query: url.Values{},

			assertErr: require.NoError,
			want:      api.InventoryAggregateFilter{},
		},
		{
			name: "include null false",
			query: url.Values{
				"server_include_null":  []string{"false"},
				"project_include_null": []string{"false"},
				"parent_include_null":  []string{"false"},
			},

			assertErr: require.NoError,
			want:      api.InventoryAggregateFilter{},
		},
		{
			name: "error - invalid server_include_null",
			query: url.Values{
				"server_include_null": []string{"invalid"},
			},

			assertErr: errassert.ValidationErrorContains(`Invalid value "invalid" for query parameter "server_include_null"`),
			want:      api.InventoryAggregateFilter{},
		},
		{
			name: "error - invalid project_include_null",
			query: url.Values{
				"project_include_null": []string{"invalid"},
			},

			assertErr: errassert.ValidationErrorContains(`Invalid value "invalid" for query parameter "project_include_null"`),
			want:      api.InventoryAggregateFilter{},
		},
		{
			name: "error - invalid parent_include_null",
			query: url.Values{
				"parent_include_null": []string{"invalid"},
			},

			assertErr: errassert.ValidationErrorContains(`Invalid value "invalid" for query parameter "parent_include_null"`),
			want:      api.InventoryAggregateFilter{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter, err := api.InventoryAggregateFilterFromURLValues(tc.query)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, filter)
		})
	}
}
