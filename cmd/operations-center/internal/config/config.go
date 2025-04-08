package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Config from global flags
	Verbose    bool `yaml:"-"`
	Debug      bool `yaml:"-"`
	ForceLocal bool `yaml:"-"`

	OperationsCenterServer string `yaml:"operations_center_server"`
}

func (c *Config) LoadConfig(path string) error {
	contents, err := os.ReadFile(filepath.Join(path, "config.yml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	return yaml.Unmarshal(contents, c)
}
