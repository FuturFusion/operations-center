package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetTokens(ctx context.Context) ([]api.Token, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(ctx, http.MethodGet, "/provisioning/tokens", query, nil)
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

func (c OperationsCenterClient) GetToken(ctx context.Context, id string) (api.Token, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/tokens", id), nil, nil)
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

func (c OperationsCenterClient) CreateToken(ctx context.Context, token api.TokenPut) error {
	response, err := c.doRequest(ctx, http.MethodPost, "/provisioning/tokens", nil, token)
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

func (c OperationsCenterClient) DeleteToken(ctx context.Context, id string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/tokens", id), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetTokenISO(ctx context.Context, id string, preseed api.TokenISOPost) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodPost, path.Join("/provisioning/tokens", id, "iso"), nil, preseed)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}
