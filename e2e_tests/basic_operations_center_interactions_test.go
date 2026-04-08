package e2e

import "testing"

func basicOperationsCenterInteractions(t *testing.T, tmpDir string) {
	t.Helper()

	assertServerRegistrationScriptletEffects(t)

	assertOperationsCenterCliAdmin(t)
	assertOperationsCenterCliQuery(t)
	assertOperationsCenterCliSystem(t)
}
