package github_test

import (
	"context"
	"io"
	"math"
	"os"
	"testing"
	"time"

	ghClient "github.com/google/go-github/v69/github"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning/repo/github"
)

func TestUpdate(t *testing.T) {
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		t.Skip(`environment variable "GITHUB_TOKEN" not set, skip this test`)
	}

	// Setup
	ctx := context.Background()
	gh := ghClient.NewClient(nil).WithAuthToken(ghToken)
	updateRepo := github.NewUpdate(gh)

	// GetAll
	updates, err := updateRepo.GetAll(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, updates)

	// GetAllIDs
	updateIDs, err := updateRepo.GetAllIDs(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, updateIDs)

	// GetByID
	update, err := updateRepo.GetByID(ctx, updateIDs[0])
	require.NoError(t, err)
	require.Equal(t, updateIDs[0], update.ID)
	require.NotEmpty(t, update.Version)
	require.True(t, update.PublishedAt.After(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))

	// GetUpdateAllFiles
	updateFiles, err := updateRepo.GetUpdateAllFiles(ctx, updateIDs[0])
	require.NoError(t, err)
	require.Greater(t, len(updateFiles), 2)
	require.Equal(t, updateIDs[0], updateFiles[0].UpdateID)
	require.NotEmpty(t, updateFiles[0].Filename)
	require.NotEmpty(t, updateFiles[0].URL.String())

	// Find smallest asset to download
	filename := updateFiles[0].Filename
	size := math.MaxInt
	for _, asset := range updateFiles {
		if asset.Size < size {
			filename = asset.Filename
			size = asset.Size
		}
	}

	// GetUpdateFileByFilename
	rc, retSize, err := updateRepo.GetUpdateFileByFilename(ctx, updateIDs[0], filename)
	require.NoError(t, err)
	defer rc.Close()
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Len(t, body, size)
	require.Equal(t, size, retSize)
}
