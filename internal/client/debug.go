package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
)

// GetPprof returns the raw output of the pprof endpoint with the given name.
func (c OperationsCenterClient) GetPprof(ctx context.Context, name string, query url.Values) (io.ReadCloser, error) {
	resp, err := c.doRequestRawResponse(ctx, http.MethodGet, path.Join("/debug/pprof", name), query, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}
