package client

import (
	"context"
	"net/http"
	"net/url"
	"path"
)

// SyncMemberImage lets the cluster member download any missing files of an image from an image source and delete the files it no longer needs.
func (c OperationsCenterClient) SyncMemberImage(ctx context.Context, sourceName string, name string, image any) error {
	_, err := c.DoInternalRequest(ctx, http.MethodPost, path.Join("/images", name, ":refresh"), url.Values{"source": []string{sourceName}}, image)
	return err
}
