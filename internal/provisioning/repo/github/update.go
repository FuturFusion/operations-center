package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-github/v69/github"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	ghOrganization     = "lxc"
	ghRepository       = "incus-os"
	ghNumberOfReleases = 20
)

type update struct {
	gh *github.Client
}

var _ provisioning.UpdateRepo = &update{}

func NewUpdate(gh *github.Client) *update {
	return &update{
		gh: gh,
	}
}

// GetAll returns a list of updates.
//
// As of now, GetAll does not actually return all updates. It has a built in
// limit to return only the 20 most recent updates.
func (u update) GetAll(ctx context.Context) (provisioning.Updates, error) {
	ghReleases, _, err := u.gh.Repositories.ListReleases(ctx, ghOrganization, ghRepository, &github.ListOptions{
		Page:    0,
		PerPage: ghNumberOfReleases,
	})
	if err != nil {
		return nil, err
	}

	updates := make(provisioning.Updates, 0, len(ghReleases))
	for _, ghRelease := range ghReleases {
		update, err := fromGHRelease(ghRelease)
		if err != nil {
			return nil, err
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// GetAllIDs returns a list of updates.
//
// As of now, GetAllIDs does not actually return all update IDs. It has a built in
// limit to return only the IDs of the 20 most recent updates.
func (u update) GetAllIDs(ctx context.Context) ([]string, error) {
	updates, err := u.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(updates))
	for _, update := range updates {
		ids = append(ids, update.ID)
	}

	return ids, nil
}

func (u update) GetByID(ctx context.Context, id string) (provisioning.Update, error) {
	ghRelease, err := u.getGHRelease(ctx, id)
	if err != nil {
		return provisioning.Update{}, err
	}

	return fromGHRelease(ghRelease)
}

func (u update) GetUpdateAllFiles(ctx context.Context, updateID string) (provisioning.UpdateFiles, error) {
	ghRelease, err := u.getGHRelease(ctx, updateID)
	if err != nil {
		return nil, err
	}

	files := make(provisioning.UpdateFiles, 0, len(ghRelease.Assets))
	for _, asset := range ghRelease.Assets {
		fileURL, err := url.Parse(ptr.From(asset.URL))
		if err != nil {
			return nil, err
		}

		files = append(files, provisioning.UpdateFile{
			UpdateID: updateID,
			Filename: ptr.From(asset.Name),
			URL:      ptr.From(fileURL),
			Size:     ptr.From(asset.Size),
		})
	}

	return files, nil
}

// GetUpdateFileByFilename downloads a file of an update.
//
// GetUpdateFileByFilename returns an io.ReadCloser that reads the contents of the specified release asset.
// It is the caller's responsibility to close the ReadCloser.
func (u update) GetUpdateFileByFilename(ctx context.Context, updateID string, filename string) (io.ReadCloser, int, error) {
	ghRelease, err := u.getGHRelease(ctx, updateID)
	if err != nil {
		return nil, 0, err
	}

	var assetID int64
	var size int
	for _, asset := range ghRelease.Assets {
		if filename == ptr.From(asset.Name) {
			assetID = ptr.From(asset.ID)
			size = ptr.From(asset.Size)
			break
		}
	}

	rc, _, err := u.gh.Repositories.DownloadReleaseAsset(ctx, ghOrganization, ghRepository, assetID, http.DefaultClient)
	if err != nil {
		return nil, 0, err
	}

	return rc, size, nil
}

func (u update) getGHRelease(ctx context.Context, id string) (*github.RepositoryRelease, error) {
	ghReleaseID, err := releaseIDFromID(id)
	if err != nil {
		return nil, err
	}

	ghRelease, _, err := u.gh.Repositories.GetRelease(ctx, ghOrganization, ghRepository, ghReleaseID)
	if err != nil {
		return nil, err
	}

	return ghRelease, nil
}

func fromGHRelease(ghRelease *github.RepositoryRelease) (provisioning.Update, error) {
	if ghRelease == nil {
		return provisioning.Update{}, fmt.Errorf("Github release is nil")
	}

	// We can not generate an ID without a ID.
	if ptr.From(ghRelease.ID) == 0 {
		return provisioning.Update{}, fmt.Errorf("Github release does not have an ID")
	}

	return provisioning.Update{
		ID:          idFromGHRelease(ghRelease),
		Version:     ptr.From(ghRelease.Name),
		PublishedAt: ghRelease.PublishedAt.Time,
		Severity:    api.UpdateSeverityNone,
	}, nil
}

const idSeparator = "$"

func idFromGHRelease(ghRelease *github.RepositoryRelease) string {
	return strings.Join([]string{ghOrganization, ghRepository, strconv.FormatInt(*ghRelease.ID, 10)}, idSeparator)
}

func releaseIDFromID(id string) (int64, error) {
	parts := strings.Split(id, idSeparator)
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid release id %q, 3 parts separated by %q expected", id, idSeparator)
	}

	releaseID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid release id %q, failed to parse final part: %w", id, err)
	}

	return releaseID, nil
}
