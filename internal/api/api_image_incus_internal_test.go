package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	imageLocalfs "github.com/FuturFusion/operations-center/internal/image/repo/localfs"
	imageRepoMock "github.com/FuturFusion/operations-center/internal/image/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestImageIncusHandler_incusImageVersionFileGet_pathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string

		wantStatusCode int
		wantBody       string
	}{
		{
			name: "success",
			path: "/1.0/images/incus/almalinux:10:amd64:cloud/20260520/incus.tar.xz",

			wantStatusCode: http.StatusOK,
			wantBody:       "metadata",
		},
		{
			name: "error - encoded path separators in version",
			path: "/1.0/images/incus/almalinux:10:amd64:cloud/20260520%2F..%2F..%2F..%2F..%2F..%2F../secret.key",

			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "error - encoded path separators in file name",
			path: "/1.0/images/incus/almalinux:10:amd64:cloud/20260520/..%2F..%2F..%2F..%2F..%2F..%2Fsecret.key",

			wantStatusCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			varDir := t.TempDir()
			err := os.WriteFile(filepath.Join(varDir, "secret.key"), []byte("secret-file-content"), 0o600)
			require.NoError(t, err)

			versionDir := filepath.Join(varDir, "images", "almalinux", "10", "amd64", "cloud", "20260520")
			err = os.MkdirAll(versionDir, 0o700)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(versionDir, "incus.tar.xz"), []byte("metadata"), 0o600)
			require.NoError(t, err)

			repo := &imageRepoMock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImage, error) {
					return &image.IncusImage{
						Name:            "almalinux:10:amd64:cloud",
						OperatingSystem: "almalinux",
						Release:         "10",
						Architecture:    "amd64",
						Variant:         "cloud",
						Versions: api.IncusImageVersions{
							"20260520": {
								Items: map[string]api.IncusImageVersionItem{
									"incus.tar.xz": {},
								},
							},
						},
					}, nil
				},
			}

			service := image.NewIncusImage(repo, imageLocalfs.New(fsroot.Open(t, filepath.Join(varDir, "images"))), nil)

			mux := http.NewServeMux()
			r := &router{mux: mux}
			registerImageIncusHandler(r.SubGroup("/authenticated"), r, nil, service)

			// Run test
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			// Assert
			require.Equal(t, tc.wantStatusCode, rec.Code, rec.Body.String())
			body, err := io.ReadAll(rec.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "secret-file-content", "files outside of the image store must never be served")
			require.Contains(t, string(body), tc.wantBody)
		})
	}
}
