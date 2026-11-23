package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	imageMock "github.com/FuturFusion/operations-center/internal/image/mock"
	imageLocalfs "github.com/FuturFusion/operations-center/internal/image/repo/localfs"
	imageRepoMock "github.com/FuturFusion/operations-center/internal/image/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/simplestreams"
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
			path: "/1.0/images/incus/local/images/almalinux:10:amd64:cloud/20260520/incus.tar.xz",

			wantStatusCode: http.StatusOK,
			wantBody:       "metadata",
		},
		{
			name: "error - encoded path separators in version",
			path: "/1.0/images/incus/local/images/almalinux:10:amd64:cloud/20260520%2F..%2F..%2F..%2F..%2F..%2F../secret.key",

			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "error - encoded path separators in file name",
			path: "/1.0/images/incus/local/images/almalinux:10:amd64:cloud/20260520/..%2F..%2F..%2F..%2F..%2F..%2Fsecret.key",

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

			versionDir := filepath.Join(varDir, "images", "local", "almalinux", "10", "amd64", "cloud", "20260520")
			err = os.MkdirAll(versionDir, 0o700)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(versionDir, "incus.tar.xz"), []byte("metadata"), 0o600)
			require.NoError(t, err)

			repo := &imageRepoMock.ImageIncusRepoMock{
				GetByNameFunc: func(ctx context.Context, source string, name string) (*image.IncusImage, error) {
					return &image.IncusImage{
						Source:          api.IncusImageSourceLocal,
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
			registerIncusImageSourcesAndImagesHandler(r.SubGroup("/authenticated"), r, nil, nil, service)

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

func TestImageIncusHandler_sourceRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		run    func(handler *imageIncusHandler, r *http.Request) response.Response

		wantStatusCode int
		wantBody       string
	}{
		{
			name:   "success - images of a source",
			source: "images",
			run: func(handler *imageIncusHandler, r *http.Request) response.Response {
				return handler.incusImagesGet(r)
			},

			wantStatusCode: http.StatusOK,
			wantBody:       "/1.0/images/incus/images/images/almalinux:10:amd64:cloud",
		},
		{
			name:   "error - images of an unknown source",
			source: "unknown",
			run: func(handler *imageIncusHandler, r *http.Request) response.Response {
				return handler.incusImagesGet(r)
			},

			wantStatusCode: http.StatusNotFound,
			wantBody:       "Not found",
		},
		{
			name:   "error - upload to a source other than local",
			source: "images",
			run: func(handler *imageIncusHandler, r *http.Request) response.Response {
				return handler.incusImageVersionPost(r)
			},

			wantStatusCode: http.StatusBadRequest,
			wantBody:       `Images can only be uploaded to the image source \"local\"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			handler := &imageIncusHandler{
				service: &imageMock.ImageIncusServiceMock{
					GetAllBySourceFunc: func(ctx context.Context, source string) (image.IncusImages, error) {
						return image.IncusImages{{Source: source, Name: "almalinux:10:amd64:cloud"}}, nil
					},
				},
				sourceService: &imageMock.IncusImageSourceServiceMock{
					GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImageSource, error) {
						if name == "unknown" {
							return nil, domain.ErrNotFound
						}

						return &image.IncusImageSource{Name: name}, nil
					},
				},
			}

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("source", tc.source)

			// Run test
			resp := tc.run(handler, r)

			// Assert
			rec := httptest.NewRecorder()
			require.NoError(t, resp.Render(rec))
			require.Equal(t, tc.wantStatusCode, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tc.wantBody, "response body")
		})
	}
}

func TestImageIncusHandler_simplestreamsPublicImagesGet(t *testing.T) {
	t.Parallel()

	newImage := func(source string, architecture string, aliases ...string) image.IncusImage {
		return image.IncusImage{
			Source:       source,
			Name:         "debian:13:" + architecture + ":default",
			Architecture: architecture,
			Aliases:      aliases,
			Versions: api.IncusImageVersions{
				"v1": {Items: map[string]api.IncusImageVersionItem{"rootfs": {}}},
			},
		}
	}

	tests := []struct {
		name   string
		images image.IncusImages

		wantAliases map[string]string
		wantPaths   map[string]string
	}{
		{
			name: "success - local wins, then the sources in the order of their ID",
			images: image.IncusImages{
				newImage("first", "amd64", "debian/13", "debian/13/first"),
				newImage("second", "amd64", "debian/13", "debian/13/first", "debian/13/second"),
				newImage(api.IncusImageSourceLocal, "amd64", "debian/13"),
				newImage(api.IncusImageSourceLocal, "arm64", "debian/13"),
			},

			wantAliases: map[string]string{
				"first:debian:13:amd64:default":  "debian/13/first",
				"second:debian:13:amd64:default": "debian/13/second",
				"local:debian:13:amd64:default":  "debian/13",
				"local:debian:13:arm64:default":  "debian/13",
			},
			wantPaths: map[string]string{
				"first:debian:13:amd64:default":  "/1.0/images/incus/first/images/debian:13:amd64:default/v1/rootfs",
				"second:debian:13:amd64:default": "/1.0/images/incus/second/images/debian:13:amd64:default/v1/rootfs",
				"local:debian:13:amd64:default":  "/1.0/images/incus/local/images/debian:13:amd64:default/v1/rootfs",
				"local:debian:13:arm64:default":  "/1.0/images/incus/local/images/debian:13:arm64:default/v1/rootfs",
			},
		},
		{
			name: "success - local image without versions is skipped and keeps no alias",
			images: image.IncusImages{
				newImage("first", "amd64", "debian/13"),
				{
					Source:       api.IncusImageSourceLocal,
					Name:         "debian:13:amd64:default",
					Architecture: "amd64",
					Aliases:      []string{"debian/13"},
					Versions:     api.IncusImageVersions{},
				},
			},

			wantAliases: map[string]string{
				"first:debian:13:amd64:default": "debian/13",
			},
			wantPaths: map[string]string{
				"first:debian:13:amd64:default": "/1.0/images/incus/first/images/debian:13:amd64:default/v1/rootfs",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			handler := &imageIncusHandler{
				service: &imageMock.ImageIncusServiceMock{
					GetAllFunc: func(ctx context.Context) (image.IncusImages, error) {
						return tc.images, nil
					},
				},
			}

			r := httptest.NewRequest(http.MethodGet, "/", nil)

			// Run test
			resp := handler.simplestreamsPublicImagesGet(r)

			// Assert
			rec := httptest.NewRecorder()
			require.NoError(t, resp.Render(rec), "render response")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var products simplestreams.Products
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &products), "decode response")

			gotAliases := map[string]string{}
			gotPaths := map[string]string{}
			for key, product := range products.Products {
				gotAliases[key] = product.Aliases
				gotPaths[key] = product.Versions["v1"].Items["rootfs"].Path
			}

			require.Equal(t, tc.wantAliases, gotAliases, "an alias of an architecture is only published for the preferred image")
			require.Equal(t, tc.wantPaths, gotPaths, "the file paths contain the source")
		})
	}
}
