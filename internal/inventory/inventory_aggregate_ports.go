package inventory

import (
	"context"
)

type InventoryAggregateService interface {
	GetAllWithFilter(ctx context.Context, filter InventoryAggregateFilter) (InventoryAggregates, error)
}

type InventoryAggregateRepo interface {
	GetAllWithFilter(ctx context.Context, filter InventoryAggregateFilter) (InventoryAggregates, error)
}
