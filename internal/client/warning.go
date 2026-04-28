package client

import (
	"context"
	"encoding/json"
	"net/http"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetWarnings(ctx context.Context) ([]api.Warning, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "/warnings", nil, nil)
	if err != nil {
		return nil, err
	}

	warnings := []api.Warning{}
	err = json.Unmarshal(response.Metadata, &warnings)
	if err != nil {
		return nil, err
	}

	return warnings, nil
}

func (c OperationsCenterClient) GetWarning(ctx context.Context, id string) (api.Warning, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, path.Join("/warnings", id), nil, nil)
	if err != nil {
		return api.Warning{}, err
	}

	warning := api.Warning{}
	err = json.Unmarshal(response.Metadata, &warning)
	if err != nil {
		return api.Warning{}, err
	}

	return warning, nil
}

func (c OperationsCenterClient) UpdateWarningStatus(ctx context.Context, id string, status api.WarningStatus) error {
	_, err := c.DoRequest(ctx, http.MethodPut, path.Join("/warnings", id), nil, api.WarningPut{
		Status: status,
	})
	if err != nil {
		return err
	}

	return nil
}
