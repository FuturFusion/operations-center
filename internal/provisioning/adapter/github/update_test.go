package github_test

import (
	"context"
	"io"
	"math"
	"os"
	"testing"

	ghClient "github.com/google/go-github/v69/github"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/github"
)

func TestUpdate(t *testing.T) {
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		t.Skip(`environment variable "GITHUB_TOKEN" not set, skip this test`)
	}

	// Setup
	ctx := context.Background()
	gh := ghClient.NewClient(nil).WithAuthToken(ghToken)
	updateRepo := github.New(gh)

	// GetLatest
	updates, err := updateRepo.GetLatest(ctx, 3)
	require.NoError(t, err)
	require.NotEmpty(t, updates)

	// GetUpdateAllFiles
	updateFiles, err := updateRepo.GetUpdateAllFiles(ctx, updates[0])
	require.NoError(t, err)
	require.Greater(t, len(updateFiles), 2)
	require.NotEmpty(t, updateFiles[0].Filename)

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
	rc, retSize, err := updateRepo.GetUpdateFileByFilenameUnverified(ctx, updates[0], filename)
	require.NoError(t, err)
	defer rc.Close()
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Len(t, body, size)
	require.Equal(t, size, retSize)
}
