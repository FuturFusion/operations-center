package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetImageIncusSources(ctx context.Context) ([]api.ImageSource, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.DoRequest(ctx, http.MethodGet, "/image/incus/sources", query, nil)
	if err != nil {
		return nil, err
	}

	imageSources := []api.ImageSource{}
	err = json.Unmarshal(response.Metadata, &imageSources)
	if err != nil {
		return nil, err
	}

	return imageSources, nil
}

func (c OperationsCenterClient) CreateImageIncusSource(ctx context.Context, newImageSource api.ImageSourcePost) error {
	_, err := c.DoRequest(ctx, http.MethodPost, path.Join("/image/incus/sources"), nil, newImageSource)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetImageIncusSource(ctx context.Context, name string) (api.ImageSource, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, path.Join("/image/incus/sources", name), nil, nil)
	if err != nil {
		return api.ImageSource{}, err
	}

	imageSource := api.ImageSource{}
	err = json.Unmarshal(response.Metadata, &imageSource)
	if err != nil {
		return api.ImageSource{}, err
	}

	return imageSource, nil
}

func (c OperationsCenterClient) DeleteImageIncusSource(ctx context.Context, name string) error {
	_, err := c.DoRequest(ctx, http.MethodDelete, path.Join("/image/incus/sources", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateImageIncusSource(ctx context.Context, name string, imageSource api.ImageSourcePut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, path.Join("/image/incus/sources", name), nil, imageSource)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RefreshImageIncusSource(ctx context.Context, name string) error {
	_, err := c.DoRequest(ctx, http.MethodPost, path.Join("/image/incus/sources", name, ":refresh"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
