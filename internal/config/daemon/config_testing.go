package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

// InitTest replaces the config singleton with one that keeps the configuration
// in memory only. The previous singleton is restored when the test ends.
func InitTest(t *testing.T, testEnv enver, saveErr error, internalConfig ...InternalConfig) {
	t.Helper()

	previousStore := defaultStore.Load()
	previousSharedStore := sharedStore.Load()
	previousInternalConfig := globalInternalConfig.Load()

	t.Cleanup(func() {
		defaultStore.Store(previousStore)
		sharedStore.Store(previousSharedStore)
		globalInternalConfig.Store(previousInternalConfig)
	})

	initInternalConfig()

	if len(internalConfig) > 0 {
		testInternalConfig := internalConfig[0]
		globalInternalConfig.Store(&testInternalConfig)
	}

	cfg := localConfig{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	require.NoError(t, err)

	defaultStore.Store(newLocalStore(testEnv, func(context.Context, localConfig) error {
		return saveErr
	}, cfg))

	shared, err := defaultShared()
	require.NoError(t, err)

	sharedStore.Store(newStore(testEnv, func(context.Context, system.SharedConfig) error {
		return saveErr
	}, notifyShared, normalizeShared, sharedConfigValidator(), shared))
}
