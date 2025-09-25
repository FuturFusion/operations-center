package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func InitTest(t *testing.T, testEnv enver, saveErr error, internalConfig ...InternalConfig) {
	t.Helper()

	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	initInternalConfig()
	if len(internalConfig) > 0 {
		globalInternalConfig = internalConfig[0]
	}

	saveFunc = func(cfg config) error {
		if saveErr != nil {
			return saveErr
		}

		globalConfigInstance = cfg

		return nil
	}

	env = testEnv

	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	require.NoError(t, err)

	globalConfigInstance = cfg
}
