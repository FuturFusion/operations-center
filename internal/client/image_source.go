package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/FuturFusion/operations-center/shared/api"
)

func (c OperationsCenterClient) GetImageSources(ctx context.Context) ([]api.ImageSource, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.DoRequest(ctx, http.MethodGet, "/image/sources", query, nil)
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

func (c OperationsCenterClient) CreateImageSource(ctx context.Context, newImageSource api.ImageSourcePost) error {
	_, err := c.DoRequest(ctx, http.MethodPost, path.Join("/image/sources"), nil, newImageSource)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetImageSource(ctx context.Context, name string) (api.ImageSource, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, path.Join("/image/sources", name), nil, nil)
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

func (c OperationsCenterClient) DeleteImageSource(ctx context.Context, name string) error {
	_, err := c.DoRequest(ctx, http.MethodDelete, path.Join("/image/sources", name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) UpdateImageSource(ctx context.Context, name string, imageSource api.ImageSourcePut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, path.Join("/image/sources", name), nil, imageSource)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) RefreshImageSource(ctx context.Context, name string) error {
	_, err := c.DoRequest(ctx, http.MethodPost, path.Join("/image/sources", name, ":refresh"), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
