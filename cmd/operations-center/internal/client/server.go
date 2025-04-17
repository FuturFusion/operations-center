package client

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetServers() ([]api.Server, error) {
	return c.GetWithFilterServers(provisioning.ServerFilter{})
}

func (c OperationsCenterClient) GetWithFilterServers(filter provisioning.ServerFilter) ([]api.Server, error) {
	query := url.Values{}
	query.Add("recursion", "1")
	query = filter.AppendToURLValues(query)

	response, err := c.doRequest(http.MethodGet, "/provisioning/servers", query, nil)
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

func (c OperationsCenterClient) GetServer(name string) (api.Server, error) {
	response, err := c.doRequest(http.MethodGet, path.Join("/provisioning/servers", name), nil, nil)
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

func (c OperationsCenterClient) CreateServer(server api.Server) error {
	content, err := json.Marshal(server)
	if err != nil {
		return err
	}

	response, err := c.doRequest(http.MethodPost, "/provisioning/servers", nil, content)
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

func (c OperationsCenterClient) DeleteServer(name string) error {
	_, err := c.doRequest(http.MethodDelete, path.Join("/provisioning/servers", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
