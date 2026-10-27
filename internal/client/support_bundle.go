package client

import (
	"context"
	"io"
	"mime"
	"net/http"

	"github.com/FuturFusion/operations-center/shared/api"
)

// GetSupportBundle returns the name suggested by the server and the content of the support bundle zip archive.
func (c OperationsCenterClient) GetSupportBundle(ctx context.Context, request api.SupportBundlePost) (string, io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodPost, "/support-bundle", nil, request)
	if err != nil {
		return "", nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return "", nil, err
	}

	filename := ""
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err == nil {
		filename = params["filename"]
	}

	return filename, resp.Body, nil
}
