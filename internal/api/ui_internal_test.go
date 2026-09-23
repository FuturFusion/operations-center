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
)

const (
	uiIndexContent = `<html>index</html>`
	uiAssetContent = `console.log(1)`
)

func TestRegisterUIHandlers(t *testing.T) {
	tests := []struct {
		name string

		embeddedUI  fs.FS
		usrShareDir func(t *testing.T) string
	}{
		{
			name: "embedded",

			embeddedUI: fstest.MapFS{
				"index.html":    &fstest.MapFile{Data: []byte(uiIndexContent)},
				"assets/app.js": &fstest.MapFile{Data: []byte(uiAssetContent)},
			},
			usrShareDir: func(t *testing.T) string {
				t.Helper()

				// The embedded web UI takes precedence, the directory does not even exist.
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
		},
		{
			name: "from directory",

			embeddedUI:  nil,
			usrShareDir: uiTestDir,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serveMux := http.NewServeMux()

			registerUIHandlers(t.Context(), newRouter(serveMux), tc.embeddedUI, tc.usrShareDir(t))

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

func uiTestDir(t *testing.T) string {
	t.Helper()

	usrShareDir := t.TempDir()
	uiDir := filepath.Join(usrShareDir, uiPathSegment)

	require.NoError(t, os.MkdirAll(filepath.Join(uiDir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "index.html"), []byte(uiIndexContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "assets", "app.js"), []byte(uiAssetContent), 0o644))

	return usrShareDir
}
