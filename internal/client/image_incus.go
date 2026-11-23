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

func (c OperationsCenterClient) GetIncusImages(ctx context.Context, source string) ([]api.IncusImage, error) {
	query := url.Values{}
	query.Add("recursion", "1")

	response, err := c.DoRequest(ctx, http.MethodGet, incusImagePath(source), query, nil)
	if err != nil {
		return nil, err
	}

	incusImages := []api.IncusImage{}
	err = json.Unmarshal(response.Metadata, &incusImages)
	if err != nil {
		return nil, err
	}

	return incusImages, nil
}

func (c OperationsCenterClient) CreateIncusImageVersion(ctx context.Context, filesReader ContentTypeReadCloser) error {
	_, err := c.DoRequest(ctx, http.MethodPost, incusImagePath(api.IncusImageSourceLocal), nil, filesReader)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetIncusImage(ctx context.Context, source string, name string) (api.IncusImage, error) {
	response, err := c.DoRequest(ctx, http.MethodGet, incusImagePath(source, name), nil, nil)
	if err != nil {
		return api.IncusImage{}, err
	}

	incusImage := api.IncusImage{}
	err = json.Unmarshal(response.Metadata, &incusImage)
	if err != nil {
		return api.IncusImage{}, err
	}

	return incusImage, nil
}

func (c OperationsCenterClient) DeleteIncusImage(ctx context.Context, source string, name string) error {
	_, err := c.DoRequest(ctx, http.MethodDelete, incusImagePath(source, name), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) DeleteIncusImageVersion(ctx context.Context, source string, name string, version string) error {
	_, err := c.DoRequest(ctx, http.MethodDelete, incusImagePath(source, name, version), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c OperationsCenterClient) GetIncusImageVersionFile(ctx context.Context, source string, name string, version string, filename string) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodGet, incusImagePath(source, name, version, filename), nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}

func (c OperationsCenterClient) UpdateIncusImage(ctx context.Context, source string, name string, incusImage api.IncusImagePut) error {
	_, err := c.DoRequest(ctx, http.MethodPut, incusImagePath(source, name), nil, incusImage)
	if err != nil {
		return err
	}

	return nil
}

func incusImagePath(source string, elements ...string) string {
	// Image names can contain characters with a special meaning in URLs.
	escaped := make([]string, 0, len(elements)+3)
	escaped = append(escaped, "/images/incus", url.PathEscape(source), "images")
	for _, element := range elements {
		escaped = append(escaped, url.PathEscape(element))
	}

	return path.Join(escaped...)
}
