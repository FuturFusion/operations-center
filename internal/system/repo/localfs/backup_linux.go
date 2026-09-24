//go:build linux

package localfs

import (
	"github.com/FuturFusion/operations-center/internal/util/file"
)

// NewBackup returns a backup repo for the state of Operations Center in varDir.
func NewBackup(varDir string) backup {
	return backup{
		varDir: varDir,
		usage: func() (file.UsageInformation, error) {
			return file.UsageInformationForPath(varDir)
		},
	}
}
