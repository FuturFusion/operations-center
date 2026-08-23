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
			segments := api.TokenSeedImagePathSegments(tc.imageType, tc.architecture, tc.channel)

			require.Equal(t, tc.wantPath, path.Join(segments...))

			baseURL, err := url.Parse("https://operations-center:7443/1.0")
			require.NoError(t, err)

			require.Equal(t, "https://operations-center:7443/1.0/"+tc.wantPath, baseURL.JoinPath(segments...).String())
		})
	}
}
