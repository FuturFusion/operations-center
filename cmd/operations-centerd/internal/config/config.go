package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	RestServerAddr string `yaml:"-"`
	RestServerPort int    `yaml:"-"`

	ClientCertificateFilename string `yaml:"-"`
	ClientKeyFilename         string `yaml:"-"`

	GithubToken string `yaml:"github.token"`
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
