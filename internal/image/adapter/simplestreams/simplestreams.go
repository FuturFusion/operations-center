package simplestreams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	incusSimplestreams "github.com/lxc/incus/v7/shared/simplestreams"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/shared/api"
)

type simplestreams struct {
	client *http.Client
}

var _ image.SimplestreamsPort = &simplestreams{}

func New() image.SimplestreamsPort {
	return &simplestreams{
		client: http.DefaultClient,
	}
}

func (s *simplestreams) GetImageList(ctx context.Context, source image.ImageSource) (image.IncusImages, error) {
	indexURL, err := url.JoinPath(source.URL, "streams/v1/index.json")
	if err != nil {
		return nil, err
	}

	indexResp, err := s.client.Get(indexURL)
	if err != nil {
		return nil, err
	}

	defer indexResp.Body.Close()

	if indexResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected status code received for image stream index: %d", indexResp.StatusCode)
	}

	simplestreamsIndex := incusSimplestreams.Stream{}
	err = json.NewDecoder(indexResp.Body).Decode(&simplestreamsIndex)
	if err != nil {
		return nil, err
	}

	var imageIndexPath string
	for _, index := range simplestreamsIndex.Index {
		if index.DataType != "image-downloads" {
			continue
		}

		imageIndexPath = index.Path
		break
	}

	if imageIndexPath == "" {
		return nil, fmt.Errorf(`Image source does not provide data type "image-downloads"`)
	}

	imageIndexURL, err := urlJoinPathAbsolute(source.URL, imageIndexPath)
	if err != nil {
		return nil, err
	}

	imageIndexResp, err := s.client.Get(imageIndexURL)
	if err != nil {
		return nil, err
	}

	defer imageIndexResp.Body.Close()

	if imageIndexResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected status code received for image stream image index: %d", imageIndexResp.StatusCode)
	}

	products := struct {
		Products map[string]api.IncusImage `json:"products"`
	}{}
	err = json.NewDecoder(imageIndexResp.Body).Decode(&products)
	if err != nil {
		return nil, err
	}

	incusImages := make(image.IncusImages, 0, len(products.Products))

	for name, product := range products.Products {
		img := image.IncusImage{
			Name:            name,
			Aliases:         product.Aliases,
			Description:     product.Description,
			OperatingSystem: product.OperatingSystem,
			Release:         product.Release,
			Architecture:    product.Architecture,
			Variant:         product.Variant,
			Source:          &source.Name,
			Versions:        product.Versions,
		}

		incusImages = append(incusImages, img)
	}

	return incusImages, nil
}

func (s *simplestreams) GetFile(ctx context.Context, source image.ImageSource, filePath string) (io.ReadCloser, error) {
	fileURL, err := urlJoinPathAbsolute(source.URL, filePath)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Get(fileURL)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func urlJoinPathAbsolute(baseHost string, filePath string) (result string, err error) {
	if !path.IsAbs(filePath) {
		// relative path
		return url.JoinPath(baseHost, filePath)
	}

	// absolute path
	baseHostURL, err := url.ParseRequestURI(baseHost)
	if err != nil {
		return "", err
	}

	baseHostURL.Path = filePath

	return baseHostURL.String(), nil
}
