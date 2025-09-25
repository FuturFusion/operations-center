package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func InitTest(t *testing.T, internalConfig ...InternalConfig) {
	t.Helper()

	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	initInternalConfig()
	if len(internalConfig) > 0 {
		globalInternalConfig = internalConfig[0]
	}

	saveFunc = saveInMemory

	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	require.NoError(t, err)

	globalConfigInstance = cfg
}

func saveInMemory(cfg config) error {
	globalConfigInstance = cfg

	return nil
}
