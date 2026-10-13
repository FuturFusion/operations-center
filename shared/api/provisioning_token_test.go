package api_test

import (
	"net/url"
	"path"
	"testing"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestTokenSeedImagePathSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		imageType    api.ImageType
		architecture images.UpdateFileArchitecture
		channel      string

		wantPath string
	}{
		{
			name: "iso without channel",

			imageType:    api.ImageTypeISO,
			architecture: images.UpdateFileArchitecture64BitX86,

			wantPath: "architecture/x86_64/type/iso/file.iso",
		},
		{
			name: "raw with channel",

			imageType:    api.ImageTypeRaw,
			architecture: images.UpdateFileArchitecture64BitARM,
			channel:      "stable",

			wantPath: "architecture/aarch64/channel/stable/type/raw/file.raw",
		},
		{
			name: "seed name and channel are escaped",

			imageType:    api.ImageTypeISO,
			architecture: images.UpdateFileArchitecture64BitX86,
			channel:      "team/beta 2",

			wantPath: "architecture/x86_64/channel/team%2Fbeta%202/type/iso/file.iso",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			segments := api.TokenSeedImagePathSegments(tc.imageType, tc.architecture, tc.channel)

			require.Equal(t, tc.wantPath, path.Join(segments...))

			baseURL, err := url.Parse("https://operations-center:7443/1.0")
			require.NoError(t, err)

			require.Equal(t, "https://operations-center:7443/1.0/"+tc.wantPath, baseURL.JoinPath(segments...).String())
		})
	}
}

func TestTokenSeedPreparedImagePathSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		imageType    api.ImageType
		architecture images.UpdateFileArchitecture
		channel      string
		deploymentID string

		wantPath string
	}{
		{
			name: "without a deployment",

			imageType:    api.ImageTypeISO,
			architecture: images.UpdateFileArchitecture64BitX86,

			wantPath: "architecture/x86_64/type/iso/a1B2c3D4e5F6.iso",
		},
		{
			name: "with a deployment",

			imageType:    api.ImageTypeISO,
			architecture: images.UpdateFileArchitecture64BitX86,
			channel:      "stable",
			deploymentID: "f6E5d4C3b2A1",

			wantPath: "architecture/x86_64/channel/stable/deployment/f6E5d4C3b2A1/type/iso/a1B2c3D4e5F6.iso",
		},
		{
			name: "a raw image of a deployment",

			imageType:    api.ImageTypeRaw,
			architecture: images.UpdateFileArchitecture64BitARM,
			deploymentID: "f6E5d4C3b2A1",

			wantPath: "architecture/aarch64/deployment/f6E5d4C3b2A1/type/raw/a1B2c3D4e5F6.raw",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			segments := api.TokenSeedPreparedImagePathSegments(tc.imageType, tc.architecture, tc.channel, tc.deploymentID, "a1B2c3D4e5F6")

			require.Equal(t, tc.wantPath, path.Join(segments...))
		})
	}
}
