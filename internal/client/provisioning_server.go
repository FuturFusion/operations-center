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
	_, err := c.doRequest(ctx, http.MethodPost, "/provisioning/servers", nil, server)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateServer(ctx context.Context, name string, server api.ServerPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/servers", name), nil, server)
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
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name), nil, api.ServerPost{
		Name: newName,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) ResyncServer(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name, ":resync"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) EvacuateServerSystem(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name, "system/:evacuate"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) PoweroffServerSystem(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name, "system/:poweroff"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RebootServerSystem(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name, "system/:reboot"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateServerSystem(ctx context.Context, name string, updateRequest api.ServerUpdatePost) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/servers", name, "system/:update"), nil, updateRequest)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetServerSystemNetwork(ctx context.Context, name string) (api.ServerSystemNetwork, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/servers", name, "system/network"), nil, nil)
	if err != nil {
		return api.ServerSystemNetwork{}, err
	}

	serverSystemNetwork := api.ServerSystemNetwork{}
	err = json.Unmarshal(response.Metadata, &serverSystemNetwork)
	if err != nil {
		return api.ServerSystemNetwork{}, err
	}

	return serverSystemNetwork, nil
}

func (c OperationsCenterClient) UpdateServerSystemNetwork(ctx context.Context, name string, server api.ServerSystemNetwork) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/servers", name, "system/network"), nil, server)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetServerSystemStorage(ctx context.Context, name string) (api.ServerSystemStorage, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/servers", name, "system/storage"), nil, nil)
	if err != nil {
		return api.ServerSystemStorage{}, err
	}

	serverSystemStorage := api.ServerSystemStorage{}
	err = json.Unmarshal(response.Metadata, &serverSystemStorage)
	if err != nil {
		return api.ServerSystemStorage{}, err
	}

	return serverSystemStorage, nil
}

func (c OperationsCenterClient) UpdateServerSystemStorage(ctx context.Context, name string, server api.ServerSystemStorage) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/servers", name, "system/storage"), nil, server)
	if err != nil {
		return err
	}

	return nil
}
