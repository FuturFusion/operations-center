package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	RestServerAddr string `yaml:"rest_server.addr"`
	RestServerPort int    `yaml:"rest_server.port"`

	ClientCertificateFilename string `yaml:"client.certificate_filename"`
	ClientKeyFilename         string `yaml:"client.key_filename"`
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
