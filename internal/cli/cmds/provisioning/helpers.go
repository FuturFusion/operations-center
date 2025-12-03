package provisioning

import (
	"bytes"
	"strings"
)

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
