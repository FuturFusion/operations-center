package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/FuturFusion/operations-center/shared/api"
)

// DeployServerOption adds an optional query parameter to a deploy request.
type DeployServerOption func(query url.Values)

// WithBMCRecording records the BMC traffic of the deployment. It is a
// development feature and not part of the documented API.
func WithBMCRecording() DeployServerOption {
	return func(query url.Values) {
		query.Set("record_bmc", "true")
	}
}

func (c OperationsCenterClient) GetServerBMCRecordings(ctx context.Context, name string) ([]api.BMCRecordingManifest, error) {
	response, err := c.DoInternalRequest(ctx, http.MethodGet, path.Join("/bmc-recordings", name), nil, nil)
	if err != nil {
		return nil, err
	}

	manifests := []api.BMCRecordingManifest{}
	err = json.Unmarshal(response.Metadata, &manifests)
	if err != nil {
		return nil, err
	}

	return manifests, nil
}

func (c OperationsCenterClient) GetServerBMCRecording(ctx context.Context, name string, id string) (*api.BMCRecordingManifest, error) {
	response, err := c.DoInternalRequest(ctx, http.MethodGet, path.Join("/bmc-recordings", name, id), nil, nil)
	if err != nil {
		return nil, err
	}

	manifest := api.BMCRecordingManifest{}
	err = json.Unmarshal(response.Metadata, &manifest)
	if err != nil {
		return nil, err
	}

	return &manifest, nil
}

// GetServerBMCRecordingArchive returns the zip of a recording. The caller
// closes the returned reader.
func (c OperationsCenterClient) GetServerBMCRecordingArchive(ctx context.Context, name string, id string, anonymize bool) (io.ReadCloser, error) {
	query := url.Values{}
	query.Set("archive", "zip")
	query.Set("anonymize", strconv.FormatBool(anonymize))

	resp, err := c.doInternalRequestRawResponse(ctx, http.MethodGet, path.Join("/bmc-recordings", name, id), query, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, err = processResponse(resp)
		return nil, err
	}

	return resp.Body, nil
}

func (c OperationsCenterClient) DeleteServerBMCRecording(ctx context.Context, name string, id string) error {
	_, err := c.DoInternalRequest(ctx, http.MethodDelete, path.Join("/bmc-recordings", name, id), nil, nil)
	if err != nil {
		return err
	}

	return nil
}
