package inventory

import (
	"context"
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

type inventoryAggregateService struct {
	repo InventoryAggregateRepo
}

var _ InventoryAggregateService = &inventoryAggregateService{}

func NewInventoryAggregateService(repo InventoryAggregateRepo) inventoryAggregateService {
	inventoryAggregateSvc := inventoryAggregateService{
		repo: repo,
	}

	return inventoryAggregateSvc
}

func (s inventoryAggregateService) GetAllWithFilter(ctx context.Context, filter InventoryAggregateFilter) (InventoryAggregates, error) {
	var filterExpression *vm.Program
	var err error

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(InventoryAggregate{})}...)
		if err != nil {
			return nil, err
		}
	}

	inventoryAggregates, err := s.repo.GetAllWithFilter(ctx, InventoryAggregateColumns{
		Servers:              true,
		Images:               true,
		Instances:            true,
		Networks:             true,
		NetworkACLs:          true,
		NetworkForwards:      true,
		NetworkIntegrations:  true,
		NetworkLoadBalancers: true,
		NetworkPeers:         true,
		NetworkZones:         true,
		Profiles:             true,
		Projects:             true,
		StorageBuckets:       true,
		StoragePools:         true,
		StorageVolumes:       true,
	}, filter)
	if err != nil {
		return nil, err
	}

	var filteredInventoryAggregates InventoryAggregates
	if filter.Expression != nil {
		for _, inventoryAggregate := range inventoryAggregates {
			output, err := expr.Run(filterExpression, inventoryAggregate)
			if err != nil {
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if result {
				filteredInventoryAggregates = append(filteredInventoryAggregates, inventoryAggregate)
			}
		}

		return filteredInventoryAggregates, nil
	}

	return inventoryAggregates, nil
}
