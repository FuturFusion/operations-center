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

func (c OperationsCenterClient) GetClusters(ctx context.Context) ([]api.Cluster, error) {
	return c.GetWithFilterClusters(ctx, provisioning.ClusterFilter{})
}

func (c OperationsCenterClient) GetWithFilterClusters(ctx context.Context, filter provisioning.ClusterFilter) ([]api.Cluster, error) {
	query := url.Values{}
	query.Add("recursion", "1")
	query = filter.AppendToURLValues(query)

	response, err := c.doRequest(ctx, http.MethodGet, "/provisioning/clusters", query, nil)
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

func (c OperationsCenterClient) GetCluster(ctx context.Context, name string) (api.Cluster, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/clusters", name), nil, nil)
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

func (c OperationsCenterClient) CreateCluster(ctx context.Context, cluster api.ClusterPost) error {
	content, err := json.Marshal(cluster)
	if err != nil {
		return err
	}

	response, err := c.doRequest(ctx, http.MethodPost, "/provisioning/clusters", nil, content)
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

func (c OperationsCenterClient) DeleteCluster(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/clusters", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) ResyncCluster(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/clusters", name, "resync-inventory"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
