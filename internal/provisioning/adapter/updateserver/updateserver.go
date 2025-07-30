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
	"github.com/FuturFusion/operations-center/shared/api"
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
	Format  string                `json:"format"`
	Updates []provisioning.Update `json:"updates"`
}

func (u updateServer) GetLatest(ctx context.Context, limit int) (provisioning.Updates, error) {
	indexURL := u.baseURL + "/index.sjson"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("GetLatest: %w", err)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to query latest updates: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
	}

	contentSig, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetLatest: %w", err)
	}

	contentVerified, err := u.verifier.Verify(contentSig)
	if err != nil {
		return nil, fmt.Errorf(`Failed to verify signature of "index.sjson": %w`, err)
	}

	updates := UpdatesIndex{}
	err = json.Unmarshal(contentVerified, &updates)
	if err != nil {
		return nil, fmt.Errorf("GetLatest: %w", err)
	}

	if updates.Format != "1.0" {
		return nil, fmt.Errorf(`Unsupported update format %q, supported formats are: "1.0"`, updates.Format)
	}

	updatesList := make([]provisioning.Update, 0, len(updates.Updates))
	for _, update := range updates.Updates {
		update.Status = api.UpdateStatusUnknown
		update.ExternalID = update.Version
		update.UUID = uuidFromUpdateServer(update)

		// Fallback to x84_64 for architecture if not defined.
		for i := range update.Files {
			if update.Files[i].Architecture == api.ArchitectureUndefined {
				update.Files[i].Architecture = api.Architecture64BitIntelX86
			}
		}

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
		updateURL := u.baseURL + "/" + path.Join(inUpdate.URL, filename)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateURL, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("GetUpdateAllFiles: %w", err)
		}

		resp, err := u.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Failed to query %q of update %q: %w", filename, inUpdate.Version, err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("GetUpdateAllFiles: %w", err)
		}

		return body, nil
	}

	contentSig, err := getFile("update.sjson")
	if err != nil {
		return nil, err
	}

	content, err := u.verifier.Verify(contentSig)
	if err != nil {
		return nil, fmt.Errorf(`Failed to verify signature of "update.sjson": %w`, err)
	}

	update := provisioning.Update{}
	err = json.Unmarshal(content, &update)
	if err != nil {
		return nil, fmt.Errorf("GetUpdateAllFiles: %w", err)
	}

	// Fallback to x84_64 for architecture if not defined.
	for i := range update.Files {
		if update.Files[i].Architecture == api.ArchitectureUndefined {
			update.Files[i].Architecture = api.Architecture64BitIntelX86
		}
	}

	return update.Files, nil
}

// GetUpdateFileByFilenameUnverified downloads a file of an update.
//
// GetUpdateFileByFilenameUnverified returns an io.ReadCloser that reads the contents of the specified release asset.
// It is the caller's responsibility to close the ReadCloser.
// It is the caller's responsibility to verify the received data, e.g. using a hash.
func (u updateServer) GetUpdateFileByFilenameUnverified(ctx context.Context, inUpdate provisioning.Update, filename string) (io.ReadCloser, int, error) {
	updateURL := u.baseURL + "/" + path.Join(inUpdate.ExternalID, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateURL, http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("GetUpdateFileByFilename: %w", err)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to get %q of update %q: %w", filename, inUpdate.Version, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("Unexpected status code received: %d", resp.StatusCode)
	}

	return resp.Body, int(resp.ContentLength), nil
}
