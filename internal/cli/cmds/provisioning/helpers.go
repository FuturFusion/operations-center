package provisioning

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/shared/api"
)

func validateImageTypeAndArchitecture(imageType string, architecture string) error {
	switch imageType {
	case api.ImageTypeISO.String(), api.ImageTypeRaw.String():
	default:
		return fmt.Errorf(`Invalid value for flag "--type": %q`, imageType)
	}

	_, ok := images.UpdateFileArchitectures[images.UpdateFileArchitecture(architecture)]
	if !ok {
		return fmt.Errorf(`Invalid value for flag "--architecture": %q`, architecture)
	}

	return nil
}

func indent(indent string, s string) string {
	lines := strings.Split(s, "\n")

	out := bytes.Buffer{}

	for _, line := range lines {
		if line == "" {
			out.WriteString("\n")
			continue
		}

		out.WriteString(indent + line + "\n")
	}

	return out.String()
}
