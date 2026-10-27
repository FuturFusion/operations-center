//go:build linux

package localfs

import (
	"os"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

// NewBackup returns a backup repo for the state of Operations Center in varRoot.
func NewBackup(varRoot *os.Root) backup {
	return backup{
		varRoot: varRoot,
		usage: func() (file.UsageInformation, error) {
			return file.UsageInformationForPath(varRoot.Name())
		},
	}
}
