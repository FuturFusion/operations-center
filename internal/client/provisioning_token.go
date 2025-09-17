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

func (c OperationsCenterClient) CreateToken(ctx context.Context, newToken api.TokenPut) error {
	response, err := c.doRequest(ctx, http.MethodPost, "/provisioning/tokens", nil, newToken)
	if err != nil {
		return err
	}

	token := api.Token{}
	err = json.Unmarshal(response.Metadata, &token)
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

func (c OperationsCenterClient) GetTokenImage(ctx context.Context, id string, preseed api.TokenImagePost) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodPost, path.Join("/provisioning/tokens", id, "image"), nil, preseed)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}

func (c OperationsCenterClient) GetTokenSeed(ctx context.Context, id string, name string) (api.TokenImages, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/tokens", id, "images", name), nil, nil)
	if err != nil {
		return api.TokenImages{}, err
	}

	tokenSeed := api.TokenImages{}
	err = json.Unmarshal(response.Metadata, &tokenSeed)
	if err != nil {
		return api.TokenImages{}, err
	}

	return tokenSeed, nil
}

func (c OperationsCenterClient) CreateTokenSeed(ctx context.Context, id string, newTokenSeed api.TokenImagesPost) error {
	response, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/tokens", id, "images"), nil, newTokenSeed)
	if err != nil {
		return err
	}

	tokenSeed := api.TokenImages{}
	err = json.Unmarshal(response.Metadata, &tokenSeed)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetTokenImageFromSeed(ctx context.Context, id string, name string) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodPost, path.Join("/provisioning/tokens", id, "images", name), nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}
