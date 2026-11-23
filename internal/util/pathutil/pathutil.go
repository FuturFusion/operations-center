package pathutil

import "strings"

// IsPathElement reports whether name is a single element of a path. Such a
// name is not empty. It is not "." or "..". It has no path separator.
// Joined to a directory, it always refers to an entry in this directory.
func IsPathElement(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}
