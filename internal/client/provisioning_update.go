package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetUpdates(ctx context.Context) ([]api.Update, error) {
	return c.GetWithFilterUpdates(ctx, provisioning.UpdateFilter{})
}

func (c OperationsCenterClient) GetWithFilterUpdates(ctx context.Context, filter provisioning.UpdateFilter) ([]api.Update, error) {
	query := url.Values{}
	query.Add("recursion", "1")
	query = filter.AppendToURLValues(query)

	response, err := c.doRequest(ctx, http.MethodGet, "/provisioning/updates", query, nil)
	if err != nil {
		return nil, err
	}

	updates := []api.Update{}
	err = json.Unmarshal(response.Metadata, &updates)
	if err != nil {
		return nil, err
	}

	return updates, nil
}

func (c OperationsCenterClient) GetUpdate(ctx context.Context, id string) (api.Update, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/updates", id), nil, nil)
	if err != nil {
		return api.Update{}, err
	}

	update := api.Update{}
	err = json.Unmarshal(response.Metadata, &update)
	if err != nil {
		return api.Update{}, err
	}

	return update, nil
}

func (c OperationsCenterClient) CreateUpdate(ctx context.Context, updateStream io.ReadCloser) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/provisioning/updates", nil, updateStream)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateUpdate(ctx context.Context, id string, update api.UpdatePut) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/updates", id), nil, update)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetUpdateFiles(ctx context.Context, id string) ([]api.UpdateFile, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/updates", id, "files"), nil, nil)
	if err != nil {
		return nil, err
	}

	updateFiles := []api.UpdateFile{}
	err = json.Unmarshal(response.Metadata, &updateFiles)
	if err != nil {
		return nil, err
	}

	return updateFiles, nil
}

func (c OperationsCenterClient) CleanupAllUpdates(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/provisioning/updates", nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RefreshUpdates(ctx context.Context, wait bool) error {
	query := url.Values{}
	query.Add("wait", strconv.FormatBool(wait))

	_, err := c.doRequest(ctx, http.MethodPost, "/provisioning/updates/:refresh", query, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetUpdatesFile(ctx context.Context, id string, filename string) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodGet, path.Join("/provisioning/updates", id, "files", filename), nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}
