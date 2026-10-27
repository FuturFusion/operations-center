//go:build linux

package flasher

import (
	"github.com/FuturFusion/operations-center/internal/util/file"
)

func (c *imageCache) usageInformation() (file.UsageInformation, error) {
	return file.UsageInformationForPath(c.root.Name())
}
