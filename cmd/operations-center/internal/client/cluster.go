package client

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetClusters() ([]api.Cluster, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(http.MethodGet, "/provisioning/clusters", query, nil)
	if err != nil {
		return nil, err
	}

	clusters := []api.Cluster{}
	err = json.Unmarshal(response.Metadata, &clusters)
	if err != nil {
		return nil, err
	}

	return clusters, nil
}

func (c OperationsCenterClient) GetCluster(name string) (api.Cluster, error) {
	response, err := c.doRequest(http.MethodGet, path.Join("/provisioning/clusters", name), nil, nil)
	if err != nil {
		return api.Cluster{}, err
	}

	cluster := api.Cluster{}
	err = json.Unmarshal(response.Metadata, &cluster)
	if err != nil {
		return api.Cluster{}, err
	}

	return cluster, nil
}

func (c OperationsCenterClient) CreateCluster(cluster api.ClusterPost) error {
	content, err := json.Marshal(cluster)
	if err != nil {
		return err
	}

	response, err := c.doRequest(http.MethodPost, "/provisioning/clusters", nil, content)
	if err != nil {
		return err
	}

	clusters := []api.Cluster{}
	err = json.Unmarshal(response.Metadata, &clusters)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteCluster(name string) error {
	_, err := c.doRequest(http.MethodDelete, path.Join("/provisioning/clusters", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) ResyncCluster(name string) error {
	_, err := c.doRequest(http.MethodPost, path.Join("/provisioning/clusters", name, "resync-inventory"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
