package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetUpdateExposechannels(ctx context.Context) ([]api.UpdateExposedchannel, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(ctx, http.MethodGet, "/provisioning/update-exposedchannels", query, nil)
	if err != nil {
		return nil, err
	}

	updateExposedchannels := []api.UpdateExposedchannel{}
	err = json.Unmarshal(response.Metadata, &updateExposedchannels)
	if err != nil {
		return nil, err
	}

	return updateExposedchannels, nil
}

func (c OperationsCenterClient) GetUpdateExposedchannel(ctx context.Context, name string) (api.UpdateExposedchannel, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/update-exposedchannels", name), nil, nil)
	if err != nil {
		return api.UpdateExposedchannel{}, err
	}

	updateExposedchannel := api.UpdateExposedchannel{}
	err = json.Unmarshal(response.Metadata, &updateExposedchannel)
	if err != nil {
		return api.UpdateExposedchannel{}, err
	}

	return updateExposedchannel, nil
}

func (c OperationsCenterClient) CreateUpdateExposedchannel(ctx context.Context, exposedchannel api.UpdateExposedchannelPost) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/provisioning/update-exposedchannels", nil, exposedchannel)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateUpdateExposedchannel(ctx context.Context, name string, exposedchannel api.UpdateExposedchannelPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/update-exposedchannels", name), nil, exposedchannel)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteUpdateExposedchannel(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/update-exposedchannels", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RenameUpdateExposedchannel(ctx context.Context, name string, newName string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/update-exposedchannels", name), nil, api.UpdateExposedchannelPost{
		Name: newName,
	})
	if err != nil {
		return err
	}

	return nil
}
