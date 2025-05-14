package client

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetUpdates() ([]api.Update, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(http.MethodGet, "/provisioning/updates", query, nil)
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

func (c OperationsCenterClient) GetUpdate(id string) (api.Update, error) {
	response, err := c.doRequest(http.MethodGet, path.Join("/provisioning/updates", id), nil, nil)
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

func (c OperationsCenterClient) GetUpdateFiles(id string) ([]api.UpdateFile, error) {
	response, err := c.doRequest(http.MethodGet, path.Join("/provisioning/updates", id, "files"), nil, nil)
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
