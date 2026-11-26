package api

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
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

const (
	uiCacheControlNoStore   = "no-store"
	uiCacheControlImmutable = "public, max-age=31536000, immutable"
	uiNotFoundBody          = "404 page not found\n"
)

func TestRegisterUIHandlers(t *testing.T) {
	t.Parallel()

	uiSources := []struct {
		name string

		embeddedUI fs.FS
		uiRoot     func(t *testing.T) *os.Root
	}{
		{
			name: "embedded",

			embeddedUI: fstest.MapFS{
				"index.html":           &fstest.MapFile{Data: []byte(uiIndexContent)},
				"assets/app.js":        &fstest.MapFile{Data: []byte(uiAssetContent)},
				"assets/nested/app.js": &fstest.MapFile{Data: []byte(uiAssetContent)},
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

	tests := []struct {
		name string

		url string

		wantStatusCode   int
		wantLocation     string
		wantCacheControl string
		wantBody         string
	}{
		{
			name: "index",

			url: "/ui/",

			wantStatusCode:   http.StatusOK,
			wantLocation:     "",
			wantCacheControl: uiCacheControlNoStore,
			wantBody:         uiIndexContent,
		},
		{
			name: "index.html redirects to directory",

			url: "/ui/index.html",

			wantStatusCode:   http.StatusMovedPermanently,
			wantLocation:     "./",
			wantCacheControl: uiCacheControlNoStore,
			wantBody:         "",
		},
		{
			name: "unknown path falls back to index (SPA routing)",

			url: "/ui/provisioning/servers",

			wantStatusCode:   http.StatusOK,
			wantLocation:     "",
			wantCacheControl: uiCacheControlNoStore,
			wantBody:         uiIndexContent,
		},
		{
			name: "asset",

			url: "/ui/assets/app.js",

			wantStatusCode:   http.StatusOK,
			wantLocation:     "",
			wantCacheControl: uiCacheControlImmutable,
			wantBody:         uiAssetContent,
		},
		{
			name: "nested asset",

			url: "/ui/assets/nested/app.js",

			wantStatusCode:   http.StatusOK,
			wantLocation:     "",
			wantCacheControl: uiCacheControlImmutable,
			wantBody:         uiAssetContent,
		},
		{
			name: "asset with trailing slash redirects to asset",

			url: "/ui/assets/app.js/",

			wantStatusCode:   http.StatusMovedPermanently,
			wantLocation:     "../app.js",
			wantCacheControl: uiCacheControlNoStore,
			wantBody:         "",
		},
		{
			name: "error - unknown asset",

			url: "/ui/assets/unknown.js",

			wantStatusCode:   http.StatusNotFound,
			wantLocation:     "",
			wantCacheControl: "",
			wantBody:         uiNotFoundBody,
		},
		{
			name: "error - assets directory",

			url: "/ui/assets/",

			wantStatusCode:   http.StatusNotFound,
			wantLocation:     "",
			wantCacheControl: "",
			wantBody:         uiNotFoundBody,
		},
		{
			name: "error - nested assets directory",

			url: "/ui/assets/nested/",

			wantStatusCode:   http.StatusNotFound,
			wantLocation:     "",
			wantCacheControl: "",
			wantBody:         uiNotFoundBody,
		},
		{
			name: "redirect to index",

			url: "/ui",

			wantStatusCode:   http.StatusMovedPermanently,
			wantLocation:     "/ui/",
			wantCacheControl: "",
			wantBody:         redirectBody(t, "/ui/"),
		},
	}

	for _, src := range uiSources {
		t.Run(src.name, func(t *testing.T) {
			t.Parallel()

			serveMux := http.NewServeMux()
			registerUIHandlers(t.Context(), newRouter(serveMux), src.embeddedUI, src.uiRoot(t))

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					// Run test
					rec := httptest.NewRecorder()
					serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

					// Assert
					require.Equal(t, tc.wantStatusCode, rec.Code)
					require.Equal(t, tc.wantLocation, rec.Header().Get("Location"))
					require.Equal(t, tc.wantCacheControl, rec.Header().Get("Cache-Control"), "browsers must not cache index.html, hashed assets are immutable")
					require.Equal(t, tc.wantBody, rec.Body.String())
				})
			}
		})
	}
}

// redirectBody returns the body http.Redirect writes for a GET request.
func redirectBody(t *testing.T, url string) string {
	t.Helper()

	rec := httptest.NewRecorder()
	http.Redirect(rec, httptest.NewRequest(http.MethodGet, "/", nil), url, http.StatusMovedPermanently)

	return rec.Body.String()
}

// uiTestDir returns a web UI directory with an index and assets.
func uiTestDir(t *testing.T) string {
	t.Helper()

	uiDir := filepath.Join(t.TempDir(), uiPathSegment)

	require.NoError(t, os.MkdirAll(filepath.Join(uiDir, "assets", "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "index.html"), []byte(uiIndexContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "assets", "app.js"), []byte(uiAssetContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "assets", "nested", "app.js"), []byte(uiAssetContent), 0o644))

	return uiDir
}

func TestRegisterUIHandlers_root(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		userAgent string

		wantStatusCode  int
		wantLocation    string
		wantContentType string
		wantBody        string
	}{
		{
			name: "browser redirects to web UI",

			userAgent: "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0",

			wantStatusCode:  http.StatusMovedPermanently,
			wantLocation:    "/ui/",
			wantContentType: "text/html; charset=utf-8",
			wantBody:        redirectBody(t, "/ui/"),
		},
		{
			name: "API client gets the API endpoints",

			userAgent: "curl/8.0",

			wantStatusCode:  http.StatusOK,
			wantLocation:    "",
			wantContentType: "application/json",
			wantBody:        `{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":["/1.0"]}` + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			serveMux := http.NewServeMux()
			registerUIHandlers(t.Context(), newRouter(serveMux), nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("User-Agent", tc.userAgent)

			// Run test
			rec := httptest.NewRecorder()
			serveMux.ServeHTTP(rec, req)

			// Assert
			require.Equal(t, tc.wantStatusCode, rec.Code)
			require.Equal(t, tc.wantLocation, rec.Header().Get("Location"))
			require.Equal(t, tc.wantContentType, rec.Header().Get("Content-Type"))
			require.Equal(t, tc.wantBody, rec.Body.String(), "a redirect must not contain the API response")
		})
	}
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

	tests := []struct {
		name string

		symlinkPath string
		url         string

		wantStatusCode int
		wantBody       string
	}{
		{
			name: "falls back to index",

			symlinkPath: "secret.txt",
			url:         "/ui/secret.txt",

			wantStatusCode: http.StatusOK,
			wantBody:       uiIndexContent,
		},
		{
			name: "error - asset not found",

			symlinkPath: "assets/secret.txt",
			url:         "/ui/assets/secret.txt",

			wantStatusCode: http.StatusNotFound,
			wantBody:       uiNotFoundBody,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			uiDir := uiTestDir(t)
			outsideFile := filepath.Join(filepath.Dir(uiDir), "secret.txt")
			require.NoError(t, os.WriteFile(outsideFile, []byte("secret-file-content"), 0o600))
			require.NoError(t, os.Symlink(outsideFile, filepath.Join(uiDir, tc.symlinkPath)))

			serveMux := http.NewServeMux()
			registerUIHandlers(t.Context(), newRouter(serveMux), nil, fsroot.Open(t, uiDir))

			// Run test
			rec := httptest.NewRecorder()
			serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

			// Assert
			require.Equal(t, tc.wantStatusCode, rec.Code)
			require.Equal(t, tc.wantBody, rec.Body.String(), "a symlink must not serve files outside of the web UI directory")
		})
	}
}

// errFS is a file system, which fails to open errPath with err. It does not
// embed fstest.MapFS, because http.FS uses its Stat method to map errors.
type errFS struct {
	files fstest.MapFS

	errPath string
	err     error
}

// Open is part of the fs.FS interface.
func (e errFS) Open(name string) (fs.File, error) {
	if name == e.errPath {
		return nil, &fs.PathError{Op: "open", Path: name, Err: e.err}
	}

	return e.files.Open(name)
}

func TestRegisterUIHandlers_openError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		errPath string
		openErr error

		wantStatusCode   int
		wantCacheControl string
		wantBody         string
	}{
		{
			name: "not exist falls back to index",

			errPath: "favicon.ico",
			openErr: fs.ErrNotExist,

			wantStatusCode:   http.StatusOK,
			wantCacheControl: uiCacheControlNoStore,
			wantBody:         uiIndexContent,
		},
		{
			name: "path escape falls back to index",

			errPath: "favicon.ico",
			openErr: errors.New(osRootPathEscapeMsg),

			wantStatusCode:   http.StatusOK,
			wantCacheControl: uiCacheControlNoStore,
			wantBody:         uiIndexContent,
		},
		{
			name: "error - permission denied",

			errPath: "favicon.ico",
			openErr: fs.ErrPermission,

			wantStatusCode:   http.StatusForbidden,
			wantCacheControl: "",
			wantBody:         "403 Forbidden\n",
		},
		{
			name: "error - asset not exist",

			errPath: "assets/app.js",
			openErr: fs.ErrNotExist,

			wantStatusCode:   http.StatusNotFound,
			wantCacheControl: "",
			wantBody:         uiNotFoundBody,
		},
		{
			name: "error - asset path escape",

			errPath: "assets/app.js",
			openErr: errors.New(osRootPathEscapeMsg),

			wantStatusCode:   http.StatusNotFound,
			wantCacheControl: "",
			wantBody:         uiNotFoundBody,
		},
		{
			name: "error - asset permission denied",

			errPath: "assets/app.js",
			openErr: fs.ErrPermission,

			wantStatusCode:   http.StatusForbidden,
			wantCacheControl: "",
			wantBody:         "403 Forbidden\n",
		},
		{
			name: "error - asset input output error",

			errPath: "assets/app.js",
			openErr: syscall.EIO,

			wantStatusCode:   http.StatusInternalServerError,
			wantCacheControl: "",
			wantBody:         "500 Internal Server Error\n",
		},
		{
			name: "error - asset too many levels of symbolic links",

			errPath: "assets/app.js",
			openErr: syscall.ELOOP,

			wantStatusCode:   http.StatusInternalServerError,
			wantCacheControl: "",
			wantBody:         "500 Internal Server Error\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			embeddedUI := errFS{
				files: fstest.MapFS{
					"index.html":    &fstest.MapFile{Data: []byte(uiIndexContent)},
					"favicon.ico":   &fstest.MapFile{Data: []byte("icon")},
					"assets/app.js": &fstest.MapFile{Data: []byte(uiAssetContent)},
				},
				errPath: tc.errPath,
				err:     tc.openErr,
			}

			serveMux := http.NewServeMux()
			registerUIHandlers(t.Context(), newRouter(serveMux), embeddedUI, nil)

			// Run test
			rec := httptest.NewRecorder()
			serveMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/"+tc.errPath, nil))

			// Assert
			require.Equal(t, tc.wantStatusCode, rec.Code)
			require.Equal(t, tc.wantCacheControl, rec.Header().Get("Cache-Control"), "error responses must not be cached")
			require.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
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
