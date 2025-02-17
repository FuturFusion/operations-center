package db

import (
	"slices"
	"strings"
)

// isTrue returns true if value is "true", "1", "yes" or "on" (case insensitive).
func isTrue(value string) bool {
	return slices.Contains([]string{"true", "1", "yes", "on"}, strings.ToLower(value))
}
