package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func InitTest(t *testing.T, testEnv enver, saveErr error) {
	t.Helper()

	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

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
