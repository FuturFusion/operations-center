package client

import (
	"context"
	"encoding/json"
	"io"
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
	response, err := c.doRequest(ctx, http.MethodPost, "/provisioning/clusters", nil, cluster)
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

func (c OperationsCenterClient) UpdateCluster(ctx context.Context, name string, cluster api.ClusterPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/clusters", name), nil, cluster)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteCluster(ctx context.Context, name string, force bool) error {
	deleteMode := api.ClusterDeleteModeNormal
	if force {
		deleteMode = api.ClusterDeleteModeForce
	}

	query := url.Values{}
	query.Add("mode", deleteMode.String())

	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/clusters", name), query, nil)
	if err != nil {
		return err
	}

	return nil
}

// FactoryResetCluster triggers a factory reset of a cluster. This operations
// removes the servers and the cluster form Operations Center inventory
// and triggers a factory reset for all the IncusOS servers.
// This operation takes up to 2 optional arguments:
//
//   - token - if present, this token will be used in the factory reset seed instead of a freshly generated token.
//   - token seed name - if present, the seed information assigned to the given token seed is used instead of the default seed.
func (c OperationsCenterClient) FactoryResetCluster(ctx context.Context, name string, args ...string) error {
	query := url.Values{}
	query.Add("mode", api.ClusterDeleteModeFactoryReset.String())

	if len(args) > 0 {
		query.Add("token", args[0])
	}

	if len(args) > 1 {
		query.Add("tokenSeedName", args[1])
	}

	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/clusters", name), query, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RenameCluster(ctx context.Context, name string, newName string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/clusters", name), nil, api.Cluster{
		Name: newName,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) ResyncCluster(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/clusters", name, ":resync-inventory"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateClusterCertificate(ctx context.Context, name string, requestBody api.ClusterCertificatePut) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/clusters", name, "certificate"), nil, requestBody)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetClusterArtifacts(ctx context.Context, clusterName string) ([]api.ClusterArtifact, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/clusters", clusterName, "artifacts"), query, nil)
	if err != nil {
		return nil, err
	}

	clusterArtifacts := []api.ClusterArtifact{}
	err = json.Unmarshal(response.Metadata, &clusterArtifacts)
	if err != nil {
		return nil, err
	}

	return clusterArtifacts, nil
}

func (c OperationsCenterClient) GetClusterArtifact(ctx context.Context, clusterName string, artifactName string) (api.ClusterArtifact, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/clusters", clusterName, "artifacts", artifactName), nil, nil)
	if err != nil {
		return api.ClusterArtifact{}, err
	}

	clusterArtifact := api.ClusterArtifact{}
	err = json.Unmarshal(response.Metadata, &clusterArtifact)
	if err != nil {
		return api.ClusterArtifact{}, err
	}

	return clusterArtifact, nil
}

func (c OperationsCenterClient) GetClusterArtifactArchive(ctx context.Context, clusterName string, artifactName string, archiveType string) (io.ReadCloser, error) {
	query := url.Values{}
	query.Add("archive", archiveType)

	resp, err := c.doRequestRawResponse(ctx, http.MethodGet, path.Join("/provisioning/clusters", clusterName, "artifacts", artifactName), query, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}

func (c OperationsCenterClient) GetClusterArtifactFile(ctx context.Context, clusterName string, artifactName string, filename string) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodGet, path.Join("/provisioning/clusters", clusterName, "artifacts", artifactName, filename), nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}
