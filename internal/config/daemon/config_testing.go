package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

// InitTest replaces the config singleton with one that keeps the configuration
// in memory only. The previous singleton is restored when the test ends.
func InitTest(t *testing.T, testEnv enver, saveErr error, internalConfig ...InternalConfig) {
	t.Helper()

	previousStore := defaultStore.Load()
	previousInternalConfig := globalInternalConfig.Load()

	t.Cleanup(func() {
		defaultStore.Store(previousStore)
		globalInternalConfig.Store(previousInternalConfig)
	})

	initInternalConfig()

	if len(internalConfig) > 0 {
		testInternalConfig := internalConfig[0]
		globalInternalConfig.Store(&testInternalConfig)
	}

	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	require.NoError(t, err)

	defaultStore.Store(newStore(testEnv, func(_ enver, _ config) error {
		return saveErr
	}, cfg))
}
