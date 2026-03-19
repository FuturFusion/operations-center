package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

func (c OperationsCenterClient) GetSystemNetworkConfig(ctx context.Context) (system.Network, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "/system/network", nil, nil)
	if err != nil {
		return system.Network{}, err
	}

	cfg := system.Network{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Network{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemNetworkConfig(ctx context.Context, cfg system.NetworkPut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, "/system/network", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemSecurityConfig(ctx context.Context) (system.Security, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "/system/security", nil, nil)
	if err != nil {
		return system.Security{}, err
	}

	cfg := system.Security{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Security{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemSecurityConfig(ctx context.Context, cfg system.SecurityPut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, "/system/security", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemSettingsConfig(ctx context.Context) (system.Settings, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "/system/settings", nil, nil)
	if err != nil {
		return system.Settings{}, err
	}

	cfg := system.Settings{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Settings{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemSettingsConfig(ctx context.Context, cfg system.SettingsPut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, "/system/settings", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemUpdatesConfig(ctx context.Context) (system.Updates, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "/system/updates", nil, nil)
	if err != nil {
		return system.Updates{}, err
	}

	cfg := system.Updates{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Updates{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) UpdateSystemUpdatesConfig(ctx context.Context, cfg system.UpdatesPut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, "/system/updates", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) SetSystemCertificate(ctx context.Context, cfg system.CertificatePost) error {
	_, err := c.DoRequest(ctx, http.MethodPost, "/system/certificate", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}
