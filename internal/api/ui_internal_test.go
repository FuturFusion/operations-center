package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

const (
	uiIndexContent = `<html>index</html>`
	uiAssetContent = `console.log(1)`
)

func TestRegisterUIHandlers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		embeddedUI fs.FS
		uiRoot     func(t *testing.T) *os.Root
	}{
		{
			name: "embedded",

			embeddedUI: fstest.MapFS{
				"index.html":    &fstest.MapFile{Data: []byte(uiIndexContent)},
				"assets/app.js": &fstest.MapFile{Data: []byte(uiAssetContent)},
			},
			uiRoot: func(t *testing.T) *os.Root {
				t.Helper()

				// The embedded web UI takes precedence. The directory is not used.
				return fsroot.Open(t, t.TempDir())
			},
		},
		{
			name: "from directory",

			embeddedUI: nil,
			uiRoot: func(t *testing.T) *os.Root {
				t.Helper()

				return fsroot.Open(t, uiTestDir(t))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serveMux := http.NewServeMux()

			registerUIHandlers(t.Context(), newRouter(serveMux), tc.embeddedUI, tc.uiRoot(t))

			tests := []struct {
				name string

				url string

				wantStatusCode int
				wantLocation   string
				wantBody       string
			}{
				{
					name: "index",

					url: "/ui/",

					wantStatusCode: http.StatusOK,
					wantBody:       uiIndexContent,
				},
				{
					name: "asset",

					url: "/ui/assets/app.js",

					wantStatusCode: http.StatusOK,
					wantBody:       uiAssetContent,
				},
				{
					name: "unknown path falls back to index (SPA routing)",

					url: "/ui/provisioning/servers",

					wantStatusCode: http.StatusOK,
					wantBody:       uiIndexContent,
				},
				{
					name: "redirect to index",

					url: "/ui",

					wantStatusCode: http.StatusMovedPermanently,
					wantLocation:   "/ui/",
				},
			}

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					rec := httptest.NewRecorder()

					serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

					require.Equal(t, tc.wantStatusCode, rec.Code)

					if tc.wantLocation != "" {
						require.Equal(t, tc.wantLocation, rec.Header().Get("Location"))
					}

					if tc.wantBody != "" {
						require.Equal(t, tc.wantBody, rec.Body.String())
					}
				})
			}
		})
	}
}

// uiTestDir returns a web UI directory with an index and an asset.
func uiTestDir(t *testing.T) string {
	t.Helper()

	uiDir := filepath.Join(t.TempDir(), uiPathSegment)

	require.NoError(t, os.MkdirAll(filepath.Join(uiDir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "index.html"), []byte(uiIndexContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "assets", "app.js"), []byte(uiAssetContent), 0o644))

	return uiDir
}

func TestRegisterUIHandlers_notAvailable(t *testing.T) {
	t.Parallel()

	// Setup
	serveMux := http.NewServeMux()
	registerUIHandlers(t.Context(), newRouter(serveMux), nil, nil)

	// Run test
	rec := httptest.NewRecorder()
	serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))

	// Assert
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegisterUIHandlers_symlinkOutsideOfUIDir(t *testing.T) {
	t.Parallel()

	// Setup
	uiDir := uiTestDir(t)
	outsideFile := filepath.Join(filepath.Dir(uiDir), "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret-file-content"), 0o600))
	require.NoError(t, os.Symlink(outsideFile, filepath.Join(uiDir, "assets", "secret.txt")))

	serveMux := http.NewServeMux()
	registerUIHandlers(t.Context(), newRouter(serveMux), nil, fsroot.Open(t, uiDir))

	// Run test
	rec := httptest.NewRecorder()
	serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/assets/secret.txt", nil))

	// Assert
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "secret-file-content", "a symlink must not serve files outside of the web UI directory")
}

func TestDaemon_openUIRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		usrShareDir func(t *testing.T) string

		assertErr    require.ErrorAssertionFunc
		assertRoot   func(t *testing.T, root *os.Root)
		wantSubRoots int
	}{
		{
			name: "success",
			usrShareDir: func(t *testing.T) string {
				t.Helper()
				return filepath.Dir(uiTestDir(t))
			},

			assertErr: require.NoError,
			assertRoot: func(t *testing.T, root *os.Root) {
				t.Helper()
				require.NotNil(t, root)
			},
			wantSubRoots: 1,
		},
		{
			name: "directory does not exist",
			usrShareDir: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},

			assertErr: require.NoError,
			assertRoot: func(t *testing.T, root *os.Root) {
				t.Helper()
				require.Nil(t, root)
			},
			wantSubRoots: 0,
		},
		{
			name: "error - not a directory",
			usrShareDir: func(t *testing.T) string {
				t.Helper()

				usrShareDir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(usrShareDir, uiPathSegment), nil, 0o644))

				return usrShareDir
			},

			assertErr: require.Error,
			assertRoot: func(t *testing.T, root *os.Root) {
				t.Helper()
				require.Nil(t, root)
			},
			wantSubRoots: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			usrShareDir := tc.usrShareDir(t)
			d := &Daemon{
				env: &mock.EnvironmentMock{
					UsrShareDirFunc: func() string {
						return usrShareDir
					},
				},
			}

			// Run test
			root, err := d.openUIRoot(t.Context())

			// Assert
			tc.assertErr(t, err)
			tc.assertRoot(t, root)
			require.Len(t, d.subRoots, tc.wantSubRoots, "the root is closed by closeRoots")
			require.NoError(t, d.closeRoots())
		})
	}
}
