package uuidgen

import (
	"strings"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testingT interface {
	require.TestingT
	Helper()
}

// FromPattern accepts a pattern, that is then repeated until the 32 bytes
// of the uuid are filled. The length of the pattern is required to be a
// divisor of 32. Only hex digits (0-9a-f) are allowed in the pattern.
// Examples:
//
//	pattern  -> uuid
//	1        -> 11111111-1111-1111-1111-111111111111
//	beef     -> beefbeef-beef-beef-beef-beefbeefbeef
//	fee1900d -> fee1900d-fee1-900d-fee1-900dfee1900d
func FromPattern(t testingT, pattern string) uuid.UUID {
	t.Helper()

	require.NotEmpty(t, pattern, "pattern for uuidgen can not be empty")

	id, err := uuid.Parse(strings.Repeat(pattern, 32/len(pattern)))
	require.NoError(t, err)

	return id
}
