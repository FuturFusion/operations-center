package client

import (
	"context"
	"encoding/json"
	"io"
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

func (c OperationsCenterClient) GetSystemBackup(ctx context.Context, options system.BackupPost) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodPost, "/system/:backup", nil, options)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}

func (c OperationsCenterClient) RestoreSystemBackup(ctx context.Context, archive io.Reader) error {
	_, err := c.DoRequest(ctx, http.MethodPost, "/system/:restore", nil, gzipArchive{Reader: archive})
	if err != nil {
		return err
	}

	return nil
}

// gzipArchive sends a gzip compressed archive.
type gzipArchive struct {
	io.Reader
}

func (gzipArchive) Close() error {
	return nil
}

func (gzipArchive) ContentType() string {
	return "application/gzip"
}
