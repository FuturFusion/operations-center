package e2e

import (
	"context"
	"testing"
)

func basicOperationsCenterInteractions(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliAdmin(t)
	assertOperationsCenterCliQuery(t)
	assertOperationsCenterCliSystem(t)
	assertOperationsCenterCliProvisioningTokenSeed(t, tmpDir)
	assertOperationsCenterCliProvisioningClusterTemplate(t, tmpDir)
}

func basicOperationsCenterInteractionsUpdatesCleanupAndRefresh(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliUpdateCleanupAndRefresh(ctx, t)
}

func registerServer(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	assertServerRegistrationScriptletEffects(t)
}
