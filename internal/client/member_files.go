package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
)

// GetMemberFile returns a file of a store on the cluster member, or a directory as a tar archive.
func (c OperationsCenterClient) GetMemberFile(ctx context.Context, store string, name string) (_ io.ReadCloser, size int64, _ error) {
	resp, err := c.doInternalRequestRawResponse(ctx, http.MethodGet, path.Join("/files", store, name), nil, nil)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		_, err = processResponse(resp)
		return nil, 0, err
	}

	return resp.Body, resp.ContentLength, nil
}

// PullMemberFiles lets the cluster member copy a directory of a store from the cluster member at source.
func (c OperationsCenterClient) PullMemberFiles(ctx context.Context, store string, name string, source string) error {
	_, err := c.DoInternalRequest(ctx, http.MethodPost, path.Join("/files", store, name), url.Values{"source": []string{source}}, nil)
	return err
}

// DeleteMemberFile deletes a file or directory of a store on the cluster member. An empty name deletes all files of the store.
func (c OperationsCenterClient) DeleteMemberFile(ctx context.Context, store string, name string) error {
	_, err := c.DoInternalRequest(ctx, http.MethodDelete, path.Join("/files", store, name), nil, nil)
	return err
}
