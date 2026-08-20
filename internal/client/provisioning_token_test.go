package client_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetTokenImageFromSeed(t *testing.T) {
	const (
		imageData = "pre-seeded-image-data"
		tokenUUID = "b32d0079-c48b-4957-b1cb-bef54125c861"
	)

	tests := []struct {
		name        string
		contentType string
		compress    bool
	}{
		{
			name:        "gzip compressed file",
			contentType: "application/gzip",
			compress:    true,
		},
		{
			name:        "gzip compressed file with media type parameters",
			contentType: "application/gzip; charset=binary",
			compress:    true,
		},
		{
			name:        "uncompressed image",
			contentType: "application/octet-stream",
			compress:    false,
		},
		{
			name:        "uncompressed image without content type",
			contentType: "",
			compress:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAccept string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.EscapedPath()
				gotAccept = r.Header.Get("Accept")

				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}

				if !tc.compress {
					_, _ = w.Write([]byte(imageData))
					return
				}

				gzipWriter := gzip.NewWriter(w)
				defer gzipWriter.Close()

				_, _ = gzipWriter.Write([]byte(imageData))
			}))
			t.Cleanup(server.Close)

			ocClient, err := client.New(server.URL)
			require.NoError(t, err)

			image, err := ocClient.GetTokenImageFromSeed(t.Context(), tokenUUID, "team-seed-1", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable")
			require.NoError(t, err)

			defer image.Close()

			body, err := io.ReadAll(image)
			require.NoError(t, err)

			require.Equal(t, "application/gzip", gotAccept)
			require.Equal(t, imageData, string(body))
			require.Equal(t, "/1.0/provisioning/tokens/"+tokenUUID+"/seeds/team-seed-1/architecture/x86_64/channel/stable/type/iso/file.iso", gotPath)
		})
	}
}
