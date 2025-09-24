package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetClusterTemplates(ctx context.Context) ([]api.ClusterTemplate, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(ctx, http.MethodGet, "/provisioning/cluster-templates", query, nil)
	if err != nil {
		return nil, err
	}

	clusterTemplates := []api.ClusterTemplate{}
	err = json.Unmarshal(response.Metadata, &clusterTemplates)
	if err != nil {
		return nil, err
	}

	return clusterTemplates, nil
}

func (c OperationsCenterClient) GetClusterTemplate(ctx context.Context, name string) (api.ClusterTemplate, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/cluster-templates", name), nil, nil)
	if err != nil {
		return api.ClusterTemplate{}, err
	}

	clusterTemplate := api.ClusterTemplate{}
	err = json.Unmarshal(response.Metadata, &clusterTemplate)
	if err != nil {
		return api.ClusterTemplate{}, err
	}

	return clusterTemplate, nil
}

func (c OperationsCenterClient) CreateClusterTemplate(ctx context.Context, clusterTemplate api.ClusterTemplatePost) error {
	response, err := c.doRequest(ctx, http.MethodPost, "/provisioning/cluster-templates", nil, clusterTemplate)
	if err != nil {
		return err
	}

	clusterTemplates := []api.ClusterTemplate{}
	err = json.Unmarshal(response.Metadata, &clusterTemplates)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteClusterTemplate(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/cluster-templates", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
