package client

import (
	"context"
	"encoding/json"
	"net/http"
	"path"

	"github.com/google/uuid"
)

// RefreshMemberUpdateFiles lets the cluster member download any missing files of an update and delete the files it no longer needs.
func (c OperationsCenterClient) RefreshMemberUpdateFiles(ctx context.Context, id uuid.UUID) error {
	_, err := c.DoInternalRequest(ctx, http.MethodPost, path.Join("/updates", id.String(), ":refresh"), nil, nil)
	return err
}

// CheckMemberUpdateFiles returns the updates whose files on the cluster member are missing or incomplete.
func (c OperationsCenterClient) CheckMemberUpdateFiles(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	resp, err := c.DoInternalRequest(ctx, http.MethodPost, "/updates/:check", nil, ids)
	if err != nil {
		return nil, err
	}

	var incomplete []uuid.UUID
	err = json.Unmarshal(resp.Metadata, &incomplete)
	if err != nil {
		return nil, err
	}

	return incomplete, nil
}
