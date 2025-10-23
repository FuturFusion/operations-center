package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func (c OperationsCenterClient) GetSystemNetworkConfig(ctx context.Context) (system.SystemNetwork, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "/system/network", nil, nil)
	if err != nil {
		return system.SystemNetwork{}, err
	}

	cfg := system.SystemNetwork{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.SystemNetwork{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemNetworkConfig(ctx context.Context, cfg system.SystemNetworkPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/system/network", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemSecurityConfig(ctx context.Context) (system.SystemSecurity, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "/system/security", nil, nil)
	if err != nil {
		return system.SystemSecurity{}, err
	}

	cfg := system.SystemSecurity{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.SystemSecurity{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemSecurityConfig(ctx context.Context, cfg system.SystemSecurityPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/system/security", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemUpdatesConfig(ctx context.Context) (system.SystemUpdates, error) {
	response, err := c.doRequest(ctx, http.MethodGet, "/system/updates", nil, nil)
	if err != nil {
		return system.SystemUpdates{}, err
	}

	cfg := system.SystemUpdates{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.SystemUpdates{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemUpdatesConfig(ctx context.Context, cfg system.SystemUpdatesPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, "/system/updates", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}
