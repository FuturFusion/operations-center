package validate

import (
	"fmt"
	"strings"
)

func FormatFlag(value string) error {
	fields := strings.SplitN(value, ",", 2)
	format := fields[0]

	var options []string
	if len(fields) == 2 {
		options = strings.Split(fields[1], ",")
		for _, option := range options {
			switch option {
			case "noheader", "header":
			default:
				return fmt.Errorf(`Invalid value for flag "--format": %q`, format)
			}
		}
	}

	switch format {
	case "csv", "json", "table", "yaml", "compact":
	default:
		return fmt.Errorf(`Invalid value for flag "--format": %q`, format)
	}

	return nil
}
