package client

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetWithFilterInventoryAggregates(filter inventory.InventoryAggregateFilter) ([]api.InventoryAggregate, error) {
	query := url.Values{}
	query = filter.AppendToURLValues(query)

	response, err := c.doRequest(http.MethodGet, "/inventory/query", query, nil)
	if err != nil {
		return nil, err
	}

	inventoryAggregates := []api.InventoryAggregate{}
	err = json.Unmarshal(response.Metadata, &inventoryAggregates)
	if err != nil {
		return nil, err
	}

	return inventoryAggregates, nil
}
