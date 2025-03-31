package inventory

import (
	"context"
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
	inventoryAggregates, err := s.repo.GetAllWithFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	return inventoryAggregates, nil
}
