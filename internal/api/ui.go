package api

import (
	"errors"
	"io/fs"
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

func registerUIHandlers(router Router, varDir string) {
	uiDir := uiHTTPDir{http.Dir(filepath.Join(varDir, uiPathSegment))}
	fileServer := http.FileServer(uiDir)

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
