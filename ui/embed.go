//go:build uiembed

package ui

import (
	"embed"
	"io/fs"
)

// dist holds the build output of the web UI.
//
// The all: prefix is required, so files, whose name starts with "." or "_" are
// embedded as well, since vite uses a base64url alphabet for the content
// hashes of the generated file names.
//
//go:embed all:dist
var dist embed.FS

// FS returns the file system holding the web UI, rooted at the directory
// containing index.html.
func FS() fs.FS {
	uiFS, _ := fs.Sub(dist, "dist")

	return uiFS
}
