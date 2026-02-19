package inventory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func TestInventoryAggregate_Filter(t *testing.T) {
	tests := []struct {
		name   string
		filter inventory.InventoryAggregateFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: inventory.InventoryAggregateFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: inventory.InventoryAggregateFilter{
				Kinds:              []string{"kind"},
				Clusters:           []string{"cluster"},
				Servers:            []string{"server"},
				ServerIncludeNull:  true,
				Projects:           []string{"project"},
				ProjectIncludeNull: true,
				Parents:            []string{"parent"},
				ParentIncludeNull:  true,
				Expression:         ptr.To("true"),
			},

			want: `cluster=cluster&filter=true&kind=kind&parent=parent&parent_include_null=true&project=project&project_include_null=true&server=server&server_include_null=true`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.String())
		})
	}
}
