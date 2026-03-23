package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetChannels(ctx context.Context) ([]api.Channel, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.DoRequest(ctx, http.MethodGet, "/provisioning/channels", query, nil)
	if err != nil {
		return nil, err
	}

	channels := []api.Channel{}
	err = json.Unmarshal(response.Metadata, &channels)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

func (c OperationsCenterClient) GetChannel(ctx context.Context, name string) (api.Channel, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, path.Join("/provisioning/channels", name), nil, nil)
	if err != nil {
		return api.Channel{}, err
	}

	channel := api.Channel{}
	err = json.Unmarshal(response.Metadata, &channel)
	if err != nil {
		return api.Channel{}, err
	}

	return channel, nil
}

func (c OperationsCenterClient) CreateChannel(ctx context.Context, channel api.ChannelPost) error {
	_, err := c.DoRequest(ctx, http.MethodPost, "/provisioning/channels", nil, channel)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateChannel(ctx context.Context, name string, channel api.ChannelPut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, path.Join("/provisioning/channels", name), nil, channel)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteChannel(ctx context.Context, name string) error {
	_, err := c.DoRequest(ctx, http.MethodDelete, path.Join("/provisioning/channels", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetChannelChangelog(ctx context.Context, name string, architecture string) (api.UpdateChangelogs, error) {
	query := url.Values{}
	query.Add("architecture", architecture)

	response, err := c.DoRequest(ctx, http.MethodGet, path.Join("/provisioning/channels", name, "changelog"), query, nil)
	if err != nil {
		return api.UpdateChangelogs{}, err
	}

	changelog := api.UpdateChangelogs{}
	err = json.Unmarshal(response.Metadata, &changelog)
	if err != nil {
		return api.UpdateChangelogs{}, err
	}

	return changelog, nil
}
