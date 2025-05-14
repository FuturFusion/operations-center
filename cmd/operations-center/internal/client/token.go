package client

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetTokens() ([]api.Token, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(http.MethodGet, "/provisioning/tokens", query, nil)
	if err != nil {
		return nil, err
	}

	tokens := []api.Token{}
	err = json.Unmarshal(response.Metadata, &tokens)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (c OperationsCenterClient) GetToken(id string) (api.Token, error) {
	response, err := c.doRequest(http.MethodGet, path.Join("/provisioning/tokens", id), nil, nil)
	if err != nil {
		return api.Token{}, err
	}

	token := api.Token{}
	err = json.Unmarshal(response.Metadata, &token)
	if err != nil {
		return api.Token{}, err
	}

	return token, nil
}

func (c OperationsCenterClient) CreateToken(token api.TokenPut) error {
	content, err := json.Marshal(token)
	if err != nil {
		return err
	}

	response, err := c.doRequest(http.MethodPost, "/provisioning/tokens", nil, content)
	if err != nil {
		return err
	}

	tokens := []api.Token{}
	err = json.Unmarshal(response.Metadata, &tokens)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteToken(id string) error {
	_, err := c.doRequest(http.MethodDelete, path.Join("/provisioning/tokens", id), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
