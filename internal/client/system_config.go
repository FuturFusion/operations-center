package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetSystemNetworkConfig(ctx context.Context) (api.SystemNetwork, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "/system/network", nil, nil)
	if err != nil {
		return api.SystemNetwork{}, err
	}

	cfg := api.SystemNetwork{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return api.SystemNetwork{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemNetworkConfig(ctx context.Context, cfg api.SystemNetworkPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/system/network", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemSecurityConfig(ctx context.Context) (api.SystemSecurity, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "/system/security", nil, nil)
	if err != nil {
		return api.SystemSecurity{}, err
	}

	cfg := api.SystemSecurity{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return api.SystemSecurity{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemSecurityConfig(ctx context.Context, cfg api.SystemSecurityPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/system/security", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemUpdatesConfig(ctx context.Context) (api.SystemUpdates, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "/system/updates", nil, nil)
	if err != nil {
		return api.SystemUpdates{}, err
	}

	cfg := api.SystemUpdates{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return api.SystemUpdates{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemUpdatesConfig(ctx context.Context, cfg api.SystemUpdatesPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/system/updates", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}
