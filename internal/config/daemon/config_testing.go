package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
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

	defaultStore.Store(newLocalStore(testEnv, func(_ context.Context, _ section, _ localConfig, newCfg localConfig) (localConfig, error) {
		return newCfg, saveErr
	}, cfg))

	shared, err := defaultShared()
	require.NoError(t, err)

	sharedStore.Store(newStore(testEnv, func(_ context.Context, _ section, _ SharedConfig, newCfg SharedConfig) (SharedConfig, error) {
		return newCfg, saveErr
	}, notifyShared, normalizeShared, sharedConfigValidator(), shared))
}
