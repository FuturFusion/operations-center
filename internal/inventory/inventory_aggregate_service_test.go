package inventory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/inventory"
	repoMock "github.com/FuturFusion/operations-center/internal/inventory/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestInventoryAggregateService_GetAllWithFilter(t *testing.T) {
	tests := []struct {
		name                    string
		filterExpression        *string
		repoGetAllWithFilter    inventory.InventoryAggregates
		repoGetAllWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success - no filter expression",
			repoGetAllWithFilter: inventory.InventoryAggregates{
				inventory.InventoryAggregate{
					Cluster: "one",
				},
				inventory.InventoryAggregate{
					Cluster: "two",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                    "error - repo",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.InventoryAggregateRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter inventory.InventoryAggregateFilter) (inventory.InventoryAggregates, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			inventoryAggregateSvc := inventory.NewInventoryAggregateService(repo)

			// Run test
			inventoryAggregate, err := inventoryAggregateSvc.GetAllWithFilter(context.Background(), inventory.InventoryAggregateFilter{
				Expression: tc.filterExpression,
			})

			// Assert
			tc.assertErr(t, err)
			require.Len(t, inventoryAggregate, tc.count)
		})
	}
}
