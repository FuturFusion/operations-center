package e2e

import "testing"

func basicOperationsCenterInteractions(t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliAdmin(t)
}
