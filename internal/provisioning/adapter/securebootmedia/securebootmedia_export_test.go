package securebootmedia

import (
	"context"
	"crypto/rsa"
)

func WithSigningKey(key *rsa.PrivateKey) Option {
	return func(m *Media) {
		m.newSigningKey = func() (*rsa.PrivateKey, error) {
			return key, nil
		}
	}
}

func WithRunTool(run func(ctx context.Context, name string, args ...string) error) Option {
	return func(m *Media) {
		m.runTool = run
	}
}

func WithLookPath(lookPath func(name string) (string, error)) Option {
	return func(m *Media) {
		m.lookPath = lookPath
	}
}

func WithBootLoaderDir(dir string) Option {
	return func(m *Media) {
		m.bootLoaderDir = dir
	}
}
