package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// emptyFS is a file system without any files.
type emptyFS struct{}

// Open is part of the fs.FS interface.
func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

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

// openUIRoot opens the "ui" directory below the usr share dir as root. It
// returns nil if the directory does not exist. closeRoots closes the root.
func (d *Daemon) openUIRoot(ctx context.Context) (*os.Root, error) {
	uiPath := filepath.Join(d.env.UsrShareDir(), uiPathSegment)

	uiRoot, err := os.OpenRoot(uiPath)
	if errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "Web UI directory does not exist", slog.String("path", uiPath), logger.Err(err))
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("Failed to open web UI directory %q: %w", uiPath, err)
	}

	d.subRoots = append(d.subRoots, uiRoot)

	return uiRoot, nil
}

// registerUIHandlers registers the handlers serving the web UI.
//
// If embeddedUI is not nil, the web UI is served from the provided file system,
// which is expected to be rooted at the directory holding index.html.
// Otherwise, the web UI is served from uiRoot. The root prevents symlinks from
// serving files outside of the web UI directory. If uiRoot is nil as well, no
// web UI is served.
func registerUIHandlers(ctx context.Context, router Router, embeddedUI fs.FS, uiRoot *os.Root) {
	var uiFS http.FileSystem

	switch {
	case embeddedUI != nil:
		slog.InfoContext(ctx, "Serving embedded web UI")

		uiFS = http.FS(embeddedUI)

	case uiRoot != nil:
		slog.InfoContext(ctx, "Serving web UI from directory", slog.String("path", uiRoot.Name()))

		uiFS = http.FS(uiRoot.FS())

	default:
		slog.WarnContext(ctx, "Web UI is not available")

		uiFS = http.FS(emptyFS{})
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
