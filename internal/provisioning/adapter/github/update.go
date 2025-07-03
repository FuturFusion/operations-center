package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/go-github/v69/github"
	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	ghOrganization = "lxc"
	ghRepository   = "incus-os"
	origin         = "github.com/lxc/incus-os"
)

var UpdateSourceSpaceUUID = uuid.MustParse(`00000000-0000-0000-0000-000000000000`)

type update struct {
	gh *github.Client
}

var _ provisioning.UpdateSourcePort = &update{}

func New(gh *github.Client) *update {
	return &update{
		gh: gh,
	}
}

// GetLatest returns a list of the latest updates available.
//
// The argument limit defines the maximum number of updates, that should be returned.
func (u update) GetLatest(ctx context.Context, limit int) (provisioning.Updates, error) {
	ghReleases, _, err := u.gh.Repositories.ListReleases(ctx, ghOrganization, ghRepository, &github.ListOptions{
		Page:    0,
		PerPage: limit,
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

func (u update) GetUpdateAllFiles(ctx context.Context, update provisioning.Update) (provisioning.UpdateFiles, error) {
	ghRelease, err := u.getGHRelease(ctx, update)
	if err != nil {
		return nil, err
	}

	files := make(provisioning.UpdateFiles, 0, len(ghRelease.Assets))
	for _, asset := range ghRelease.Assets {
		filename := ptr.From(asset.Name)

		var fileComponent api.UpdateFileComponent
		var fileType api.UpdateFileType

		switch {
		case filename == "debug.raw.gz":
			fileComponent = api.UpdateFileComponentDebug
			fileType = api.UpdateFileTypeApplication
		case filename == "incus.raw.gz":
			fileComponent = api.UpdateFileComponentIncus
			fileType = api.UpdateFileTypeApplication
		case strings.HasSuffix(filename, ".efi.gz"):
			fileComponent = api.UpdateFileComponentOS
			fileType = api.UpdateFileTypeUpdateEFI
		case strings.HasSuffix(filename, ".img.gz"):
			fileComponent = api.UpdateFileComponentOS
			fileType = api.UpdateFileTypeImageRaw
		case strings.HasSuffix(filename, ".iso.gz"):
			fileComponent = api.UpdateFileComponentOS
			fileType = api.UpdateFileTypeImageISO
		case strings.Contains(filename, ".usr-x86-64-verity."):
			fileComponent = api.UpdateFileComponentOS
			fileType = api.UpdateFileTypeUpdateUsrVerity
		case strings.Contains(filename, ".usr-x86-64-verity-sig."):
			fileComponent = api.UpdateFileComponentOS
			fileType = api.UpdateFileTypeUpdateUsrVeritySignature
		case strings.Contains(filename, ".usr-x86-64."):
			fileComponent = api.UpdateFileComponentOS
			fileType = api.UpdateFileTypeUpdateUsr
		default:
			continue
		}

		files = append(files, provisioning.UpdateFile{
			Filename:  filename,
			Size:      ptr.From(asset.Size),
			Component: fileComponent,
			Type:      fileType,

			// Fallback to x84_64 for architecture.
			Architecture: api.Architecture64BitIntelX86,
		})
	}

	return files, nil
}

// GetUpdateFileByFilename downloads a file of an update.
//
// GetUpdateFileByFilename returns an io.ReadCloser that reads the contents of the specified release asset.
// It is the caller's responsibility to close the ReadCloser.
func (u update) GetUpdateFileByFilename(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
	ghRelease, err := u.getGHRelease(ctx, update)
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

func (u update) getGHRelease(ctx context.Context, update provisioning.Update) (*github.RepositoryRelease, error) {
	ghReleaseID, err := releaseIDFromID(update.ExternalID)
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
		UUID:        uuidFromGHRelease(ghRelease),
		Origin:      origin,
		ExternalID:  externalIDFromGHRelease(ghRelease),
		Version:     ptr.From(ghRelease.Name),
		PublishedAt: ghRelease.PublishedAt.Time,
		Severity:    api.UpdateSeverityNone,
		Channel:     "daily",
		Changelog:   ptr.From(ghRelease.Body),
	}, nil
}

const idSeparator = ":"

func externalIDFromGHRelease(ghRelease *github.RepositoryRelease) string {
	return strings.Join([]string{ghOrganization, ghRepository, strconv.FormatInt(*ghRelease.ID, 10)}, idSeparator)
}

func uuidFromGHRelease(ghRelease *github.RepositoryRelease) uuid.UUID {
	identifier := strings.Join([]string{
		ghOrganization,
		ghRepository,
		strconv.FormatInt(*ghRelease.ID, 10),
	}, idSeparator)

	return uuid.NewSHA1(UpdateSourceSpaceUUID, []byte(identifier))
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
