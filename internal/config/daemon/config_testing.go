package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func InitTest(t *testing.T, testEnv enver) {
	t.Helper()

	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	saveFunc = saveInMemory

	env = testEnv

	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	require.NoError(t, err)

	globalConfigInstance = cfg
}

func saveInMemory(cfg config) error {
	globalConfigInstance = cfg

	return nil
}
