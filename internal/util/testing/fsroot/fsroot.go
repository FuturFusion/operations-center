// Package fsroot provides test helpers for os.Root.
package fsroot

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Open opens dir as root. The root is closed when the test ends.
func Open(t *testing.T, dir string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = root.Close()
	})

	return root
}
