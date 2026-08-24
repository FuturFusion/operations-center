//go:build linux

package localfs

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

func (l localfs) UsageInformation(_ context.Context) (file.UsageInformation, error) {
	return file.UsageInformationForPath(l.storageDir)
}
