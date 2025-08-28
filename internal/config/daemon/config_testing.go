package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func InitTest(t *testing.T) {
	t.Helper()

	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	isTesting = true

	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	require.NoError(t, err)

	cfg.Network.OperationsCenterAddress = "https://127.0.0.1:443"
	cfg.Updates.SourcePollSkipFirst = true

	globalConfigInstance = cfg
}
