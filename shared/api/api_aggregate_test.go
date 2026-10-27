package api_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestInventoryAggregateFilter_AppendToURLValues(t *testing.T) {
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

			require.Equal(t, tc.want, query.Encode())
		})
	}
}
