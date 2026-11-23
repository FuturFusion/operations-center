package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// GetSystemNetworkConfig returns the network config and the ETag of its part shared by the cluster members.
func (c OperationsCenterClient) GetSystemNetworkConfig(ctx context.Context) (system.Network, string, error) {
	response, etag, err := c.doRequestETag(ctx, http.MethodGet, "/system/network", nil, "")
	if err != nil {
		return system.Network{}, "", err
	}

	cfg := system.Network{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Network{}, "", err
	}

	return cfg, etag, nil
}

// UpdateSystemNetworkConfig updates the network config, if the ETag is not empty only while its shared part is unchanged.
func (c OperationsCenterClient) UpdateSystemNetworkConfig(ctx context.Context, cfg system.NetworkPut, etag string) error {
	_, _, err := c.doRequestETag(ctx, http.MethodPut, "/system/network", cfg, etag)
	if err != nil {
		return err
	}

	return nil
}

// GetSystemSecurityConfig returns the security config and the ETag of its part shared by the cluster members.
func (c OperationsCenterClient) GetSystemSecurityConfig(ctx context.Context) (system.Security, string, error) {
	response, etag, err := c.doRequestETag(ctx, http.MethodGet, "/system/security", nil, "")
	if err != nil {
		return system.Security{}, "", err
	}

	cfg := system.Security{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Security{}, "", err
	}

	return cfg, etag, nil
}

// UpdateSystemSecurityConfig updates the security config, if the ETag is not empty only while its shared part is unchanged.
func (c OperationsCenterClient) UpdateSystemSecurityConfig(ctx context.Context, cfg system.SecurityPut, etag string) error {
	_, _, err := c.doRequestETag(ctx, http.MethodPut, "/system/security", cfg, etag)
	if err != nil {
		return err
	}

	return nil
}

// GetSystemSettingsConfig returns the settings config and the ETag of its part shared by the cluster members.
func (c OperationsCenterClient) GetSystemSettingsConfig(ctx context.Context) (system.Settings, string, error) {
	response, etag, err := c.doRequestETag(ctx, http.MethodGet, "/system/settings", nil, "")
	if err != nil {
		return system.Settings{}, "", err
	}

	cfg := system.Settings{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Settings{}, "", err
	}

	return cfg, etag, nil
}

// UpdateSystemSettingsConfig updates the settings config, if the ETag is not empty only while its shared part is unchanged.
func (c OperationsCenterClient) UpdateSystemSettingsConfig(ctx context.Context, cfg system.SettingsPut, etag string) error {
	_, _, err := c.doRequestETag(ctx, http.MethodPut, "/system/settings", cfg, etag)
	if err != nil {
		return err
	}

	return nil
}

// GetSystemUpdatesConfig returns the updates config and the ETag of its part shared by the cluster members.
func (c OperationsCenterClient) GetSystemUpdatesConfig(ctx context.Context) (system.Updates, string, error) {
	response, etag, err := c.doRequestETag(ctx, http.MethodGet, "/system/updates", nil, "")
	if err != nil {
		return system.Updates{}, "", err
	}

	cfg := system.Updates{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Updates{}, "", err
	}

	return cfg, etag, nil
}

// UpdateSystemUpdatesConfig updates the updates config, if the ETag is not empty only while its shared part is unchanged.
func (c OperationsCenterClient) UpdateSystemUpdatesConfig(ctx context.Context, cfg system.UpdatesPut, etag string) error {
	_, _, err := c.doRequestETag(ctx, http.MethodPut, "/system/updates", cfg, etag)
	if err != nil {
		return err
	}

	return nil
}

// GetSystemClusterConfig returns the cluster config and its ETag.
func (c OperationsCenterClient) GetSystemClusterConfig(ctx context.Context) (system.Cluster, string, error) {
	response, etag, err := c.doRequestETag(ctx, http.MethodGet, "/system/cluster", nil, "")
	if err != nil {
		return system.Cluster{}, "", err
	}

	cfg := system.Cluster{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Cluster{}, "", err
	}

	return cfg, etag, nil
}

// UpdateSystemClusterConfig updates the cluster config, if the ETag is not empty only while it is unchanged.
func (c OperationsCenterClient) UpdateSystemClusterConfig(ctx context.Context, cfg system.ClusterPut, etag string) error {
	_, _, err := c.doRequestETag(ctx, http.MethodPut, "/system/cluster", cfg, etag)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetSystemCertificate(ctx context.Context) (system.Certificate, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, "/system/certificate", nil, nil)
	if err != nil {
		return system.Certificate{}, err
	}

	cfg := system.Certificate{}
	err = json.Unmarshal(response.Metadata, &cfg)
	if err != nil {
		return system.Certificate{}, err
	}

	return cfg, nil
}

func (c OperationsCenterClient) SetSystemCertificate(ctx context.Context, cfg system.CertificatePost) error {
	_, err := c.DoRequest(ctx, http.MethodPost, "/system/certificate", nil, cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RenewSystemCertificate(ctx context.Context) error {
	_, err := c.DoRequest(ctx, http.MethodPost, "/system/certificate/:renew", nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) CleanSystemCache(ctx context.Context) error {
	_, err := c.DoRequest(ctx, http.MethodPost, "/system/:clean-cache", nil, nil)
	if err != nil {
		return err
	}

	return nil
}

// doRequestETag sends a request with the ETag as If-Match header, if not empty, and returns the ETag of the response.
func (c OperationsCenterClient) doRequestETag(ctx context.Context, method string, endpoint string, content any, etag string) (*api.Response, string, error) {
	resp, err := c.doRequestRawResponse(ctx, method, endpoint, nil, content, func(req *http.Request) {
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
	})
	if err != nil {
		return nil, "", err
	}

	response, err := processResponse(resp)
	if err != nil {
		return nil, "", err
	}

	return response, resp.Header.Get("ETag"), nil
}
