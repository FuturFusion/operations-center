//go:build !uiembed

package ui

import "io/fs"

// FS returns nil, since the web UI has not been embedded into the binary.
func FS() fs.FS {
	return nil
}
