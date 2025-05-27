package updateserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/signature"
)

var UpdateSourceSpaceUUID = uuid.MustParse(`00000000-0000-0000-0000-000000000002`)

type updateServer struct {
	baseURL  string
	client   *http.Client
	verifier signature.Verifier
}

var _ provisioning.UpdateSourcePort = &updateServer{}

func New(baseURL string, verifier signature.Verifier) *updateServer {
	return &updateServer{
		// Normalize URL, remove trailing slash.
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		client:   http.DefaultClient,
		verifier: verifier,
	}
}

type UpdatesIndex struct {
	ContentID string                         `json:"content_id"`
	DataType  string                         `json:"datatype"`
	Format    string                         `json:"format"`
	Updates   map[string]provisioning.Update `json:"updates"`
	Updated   string                         `json:"updated,omitempty"`
}

func (u updateServer) GetLatest(ctx context.Context, limit int) (provisioning.Updates, error) {
	indexURL := u.baseURL + "/updates.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	updates := UpdatesIndex{}
	err = json.Unmarshal(body, &updates)
	if err != nil {
		return nil, err
	}

	if updates.Format != "updates:1.0" {
		return nil, fmt.Errorf(`Unsupported stream update format %q, supported formats are: "updates:1.0"`, updates.Format)
	}

	// TODO: Should the origin property from the updates.json be verified against baseURL?
	// Should we allow for mirror servers, which would have a different baseURL but serving the content from the origin origin?

	updatesList := make([]provisioning.Update, 0, len(updates.Updates))
	for updateID, update := range updates.Updates {
		update.ExternalID = updateID
		update.UUID = uuidFromUpdateServer(update)
		updatesList = append(updatesList, update)
	}

	sort.Slice(updatesList, func(i, j int) bool {
		return updatesList[i].PublishedAt.After(updatesList[j].PublishedAt)
	})

	limit = min(len(updatesList), limit)

	return provisioning.Updates(updatesList[:limit]), nil
}

const idSeparator = ":"

func uuidFromUpdateServer(update provisioning.Update) uuid.UUID {
	identifier := strings.Join([]string{
		update.Origin,
		update.Channel,
		update.Version,
		update.PublishedAt.String(),
	}, idSeparator)

	return uuid.NewSHA1(UpdateSourceSpaceUUID, []byte(identifier))
}

func (u updateServer) GetUpdateAllFiles(ctx context.Context, inUpdate provisioning.Update) (provisioning.UpdateFiles, error) {
	getFile := func(filename string) ([]byte, error) {
		updateURL := u.baseURL + "/" + path.Join(inUpdate.ExternalID, filename)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateURL, http.NoBody)
		if err != nil {
			return nil, err
		}

		resp, err := u.client.Do(req)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		return body, nil
	}

	content, err := getFile("update.json")
	if err != nil {
		return nil, err
	}

	contentSig, err := getFile("update.json.sig")
	if err != nil {
		return nil, err
	}

	err = u.verifier.Verify(content, contentSig)
	if err != nil {
		return nil, err
	}

	update := provisioning.Update{}
	err = json.Unmarshal(content, &update)
	if err != nil {
		return nil, err
	}

	return update.Files, nil
}

func (u updateServer) GetUpdateFileByFilename(ctx context.Context, inUpdate provisioning.Update, filename string) (io.ReadCloser, int, error) {
	// FIXME: Verify signature of updates.json and use the checksum from updates.json
	// to verify the integrity of the downloaded file.
	updateURL := u.baseURL + "/" + path.Join(inUpdate.ExternalID, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateURL, http.NoBody)
	if err != nil {
		return nil, 0, err
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
	}

	return resp.Body, int(resp.ContentLength), nil
}
