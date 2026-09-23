// Package ui provides access to the web UI assets of Operations Center.
//
// By default, the package embeds nothing and FS returns nil, which makes the
// daemon serve the web UI from disk (/usr/share/operations-center/ui).
//
// When built with the "uiembed" build tag, the contents of ui/dist are compiled
// into the binary and FS returns a file system rooted at the directory holding
// index.html.
//
// ui/dist has to exist when building with the uiembed build tag, otherwise the
// build fails with "pattern all:dist: no matching files found".
package ui
