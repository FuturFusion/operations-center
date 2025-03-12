package client

import (
	"encoding/json"
	"net/http"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetServers() ([]api.Server, error) {
	response, err := c.doRequest(http.MethodGet, "/provisioning/servers", "recursion=1", nil)
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
	response, err := c.doRequest(http.MethodGet, path.Join("/provisioning/servers", name), "", nil)
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

	response, err := c.doRequest(http.MethodPost, "/provisioning/servers", "", content)
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
	_, err := c.doRequest(http.MethodDelete, path.Join("/provisioning/servers", name), "", nil)
	if err != nil {
		return err
	}

	return nil
}
