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

func (c OperationsCenterClient) GetTokenSeeds(ctx context.Context, id string) ([]api.TokenSeed, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/tokens", id, "seeds"), query, nil)
	if err != nil {
		return nil, err
	}

	tokenSeeds := []api.TokenSeed{}
	err = json.Unmarshal(response.Metadata, &tokenSeeds)
	if err != nil {
		return nil, err
	}

	return tokenSeeds, nil
}

func (c OperationsCenterClient) GetTokenSeed(ctx context.Context, id string, name string) (api.TokenSeed, error) {
	response, err := c.doRequest(ctx, http.MethodGet, path.Join("/provisioning/tokens", id, "seeds", name), nil, nil)
	if err != nil {
		return api.TokenSeed{}, err
	}

	tokenSeed := api.TokenSeed{}
	err = json.Unmarshal(response.Metadata, &tokenSeed)
	if err != nil {
		return api.TokenSeed{}, err
	}

	return tokenSeed, nil
}

func (c OperationsCenterClient) CreateTokenSeed(ctx context.Context, id string, newTokenSeed api.TokenSeedPost) error {
	response, err := c.doRequest(ctx, http.MethodPost, path.Join("/provisioning/tokens", id, "seeds"), nil, newTokenSeed)
	if err != nil {
		return err
	}

	tokenSeed := api.TokenSeed{}
	err = json.Unmarshal(response.Metadata, &tokenSeed)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateTokenSeed(ctx context.Context, id string, name string, tokenSeed api.TokenSeedPut) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/provisioning/tokens", id, "seeds", name), nil, tokenSeed)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteTokenSeed(ctx context.Context, id string, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path.Join("/provisioning/tokens", id, "seeds", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetTokenImageFromSeed(ctx context.Context, id string, name string, imageType api.ImageType) (io.ReadCloser, error) {
	query := url.Values{}
	query.Add("type", imageType.String())

	resp, err := c.doRequestRawResponse(ctx, http.MethodGet, path.Join("/provisioning/tokens", id, "seeds", name), query, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}
