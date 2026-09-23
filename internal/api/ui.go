package api

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
)

type uiHTTPDir struct {
	http.FileSystem
}

const uiPathSegment = "ui"

// Open is part of the http.FileSystem interface.
func (httpFS uiHTTPDir) Open(name string) (http.File, error) {
	fsFile, err := httpFS.FileSystem.Open(name)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return httpFS.FileSystem.Open("index.html")
	}

	return fsFile, err
}

// registerUIHandlers registers the handlers serving the web UI.
//
// If embeddedUI is not nil, the web UI is served from the provided file system,
// which is expected to be rooted at the directory holding index.html.
// Otherwise, the web UI is served from the "ui" directory below usrShareDir.
func registerUIHandlers(ctx context.Context, router Router, embeddedUI fs.FS, usrShareDir string) {
	var uiFS http.FileSystem

	if embeddedUI != nil {
		slog.InfoContext(ctx, "Serving embedded web UI")

		uiFS = http.FS(embeddedUI)
	} else {
		uiPath := filepath.Join(usrShareDir, uiPathSegment)

		slog.InfoContext(ctx, "Serving web UI from directory", slog.String("path", uiPath))

		uiFS = http.Dir(uiPath)
	}

	fileServer := http.FileServer(uiHTTPDir{uiFS})

	router.Handle("GET /"+uiPathSegment+"/", http.StripPrefix("/"+uiPathSegment+"/", fileServer))
	router.HandleFunc("GET /"+uiPathSegment, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/"+uiPathSegment+"/", http.StatusMovedPermanently)
	})

	router.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if strings.Contains(ua, "Gecko") {
			// Web browser handling.
			http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = rootHandler(r).Render(w)
	})
}
