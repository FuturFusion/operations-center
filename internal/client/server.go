package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetServers(ctx context.Context) ([]api.Server, error) {
	return c.GetWithFilterServers(ctx, provisioning.ServerFilter{})
}

func (c OperationsCenterClient) GetWithFilterServers(ctx context.Context, filter provisioning.ServerFilter) ([]api.Server, error) {
	query := url.Values{}
	query.Add("recursion", "1")
	query = filter.AppendToURLValues(query)

	response, err := c.doRequest(ctx, http.MethodGet, "/provisioning/servers", query, nil)
	if err != nil {
		return nil, err
	}

	servers := []api.Server{}
	err = json.Unmarshal(response.Metadata, &servers)
	if err != nil {
		return nil, err
	}

	return servers, nil
}

func (c OperationsCenterClient) GetServer(ctx context.Context, name string) (api.Server, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/servers", name), nil, nil)
	if err != nil {
		return api.Server{}, err
	}

	server := api.Server{}
	err = json.Unmarshal(response.Metadata, &server)
	if err != nil {
		return api.Server{}, err
	}

	return server, nil
}

func (c OperationsCenterClient) CreateServer(ctx context.Context, server api.Server) error {
	response, err := c.doRequest(ctx, http.MethodPost, "/provisioning/servers", nil, server)
	if err != nil {
		return err
	}

	servers := []api.Server{}
	err = json.Unmarshal(response.Metadata, &servers)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteServer(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/servers", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RenameServer(ctx context.Context, name string, newName string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name), nil, api.Server{
		Name: newName,
	})
	if err != nil {
		return err
	}

	return nil
}
