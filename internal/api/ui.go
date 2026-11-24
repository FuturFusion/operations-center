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
	if isUIAssetPath(name) {
		return httpFS.openAsset(name)
	}

	fsFile, err := httpFS.FileSystem.Open(name)
	if errors.Is(err, fs.ErrNotExist) || isPathEscapeError(err) {
		return httpFS.FileSystem.Open("index.html")
	}

	return fsFile, err
}

// openAsset does not fall back to index.html. A missing asset returns 404, so
// the browser does not get HTML for a JavaScript or CSS file. Directories are
// hidden to prevent a directory listing.
func (httpFS uiHTTPDir) openAsset(name string) (http.File, error) {
	notExistErr := &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}

	fsFile, err := httpFS.FileSystem.Open(name)
	if isPathEscapeError(err) {
		return nil, notExistErr
	}

	if err != nil {
		return nil, err
	}

	info, err := fsFile.Stat()
	if err != nil {
		_ = fsFile.Close()
		return nil, err
	}

	if info.IsDir() {
		_ = fsFile.Close()
		return nil, notExistErr
	}

	return fsFile, nil
}

// osRootPathEscapeMsg is the message of the unexported error os.Root returns
// for paths, which point outside of the root.
const osRootPathEscapeMsg = "path escapes from parent"

// isPathEscapeError reports if err is the os.Root error for a symlink, which
// points outside of the root. The error is not exported, so compare the
// message.
func isPathEscapeError(err error) bool {
	var pathErr *fs.PathError
	return errors.As(err, &pathErr) && pathErr.Err != nil && pathErr.Err.Error() == osRootPathEscapeMsg
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

const uiAssetsDir = "assets"

// isUIAssetPath reports if name is below the assets directory. Vite puts a
// content hash in the file names of the assets.
func isUIAssetPath(name string) bool {
	name = strings.TrimPrefix(name, "/")
	return name == uiAssetsDir || strings.HasPrefix(name, uiAssetsDir+"/")
}

// uiCacheControl sets the Cache-Control header before next writes the
// response. Browsers must never cache index.html. Otherwise, a browser keeps
// running an old web UI after an update or a rollback. Hashed assets never
// change. On errors, http.FileServer removes the header.
func uiCacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cacheControl := "no-store"
		if isUIAssetPath(r.URL.Path) && !strings.HasSuffix(r.URL.Path, "/") {
			cacheControl = "public, max-age=31536000, immutable"
		}

		w.Header().Set("Cache-Control", cacheControl)
		next.ServeHTTP(w, r)
	})
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

	fileServer := uiCacheControl(http.FileServer(uiHTTPDir{uiFS}))

	router.Handle("GET /"+uiPathSegment+"/", http.StripPrefix("/"+uiPathSegment+"/", fileServer))
	router.HandleFunc("GET /"+uiPathSegment, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/"+uiPathSegment+"/", http.StatusMovedPermanently)
	})

	router.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if strings.Contains(ua, "Gecko") {
			// Web browser handling.
			http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = rootHandler(r).Render(w)
	})
}
