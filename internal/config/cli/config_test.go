package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/cli"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestConfig_LoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup prepares the directory and returns the path to load.
		setup func(t *testing.T, dir string) string

		assertErr         require.ErrorAssertionFunc
		wantConfigDir     func(path string) string
		wantCertInfo      require.ValueAssertionFunc
		wantDefaultRemote string
		wantRemotes       map[string]config.Remote
	}{
		{
			name: "success - no config file",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				return dir
			},

			assertErr:         require.NoError,
			wantConfigDir:     func(path string) string { return path },
			wantCertInfo:      require.NotNil,
			wantDefaultRemote: "",
			wantRemotes:       nil,
		},
		{
			name: "success - config file with remotes",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				writeFile(t, filepath.Join(dir, "config.yml"), `
default_remote: tls
remotes:
  default:
    addr: https://default:8443
  tls:
    addr: https://tls:8443
    auth_type: tls
  oidc:
    addr: https://oidc:8443
    auth_type: oidc
`)

				return dir
			},

			assertErr:         require.NoError,
			wantConfigDir:     func(path string) string { return path },
			wantCertInfo:      require.NotNil,
			wantDefaultRemote: "tls",
			wantRemotes: map[string]config.Remote{
				"default": {Addr: "https://default:8443", AuthType: config.AuthTypeUntrusted},
				"tls":     {Addr: "https://tls:8443", AuthType: config.AuthTypeTLS},
				"oidc":    {Addr: "https://oidc:8443", AuthType: config.AuthTypeOIDC},
			},
		},
		{
			name: "error - invalid auth type",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				writeFile(t, filepath.Join(dir, "config.yml"), `
remotes:
  invalid:
    addr: https://invalid:8443
    auth_type: invalid
`)

				return dir
			},

			assertErr:         errassert.Contains("Invalid value for config key auth_type: invalid"),
			wantConfigDir:     func(path string) string { return path },
			wantCertInfo:      require.NotNil,
			wantDefaultRemote: "",
			wantRemotes: map[string]config.Remote{
				"invalid": {Addr: "https://invalid:8443", AuthType: "invalid"},
			},
		},
		{
			name: "error - invalid YAML",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				writeFile(t, filepath.Join(dir, "config.yml"), `remotes: [`)

				return dir
			},

			assertErr:         require.Error,
			wantConfigDir:     func(path string) string { return path },
			wantCertInfo:      require.NotNil,
			wantDefaultRemote: "",
			wantRemotes:       nil,
		},
		{
			name: "error - config file is a directory",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				require.NoError(t, os.Mkdir(filepath.Join(dir, "config.yml"), 0o700))

				return dir
			},

			assertErr:         errassert.Contains("is a directory"),
			wantConfigDir:     func(path string) string { return path },
			wantCertInfo:      require.NotNil,
			wantDefaultRemote: "",
			wantRemotes:       nil,
		},
		{
			name: "error - client certificate can not be created",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				require.NoError(t, os.Mkdir(filepath.Join(dir, "client.crt"), 0o700))

				return dir
			},

			assertErr:         errassert.Contains("Failed to create client certificate"),
			wantConfigDir:     func(path string) string { return path },
			wantCertInfo:      require.Nil,
			wantDefaultRemote: "",
			wantRemotes:       nil,
		},
		{
			name: "error - config dir is a file",
			setup: func(t *testing.T, dir string) string {
				t.Helper()

				path := filepath.Join(dir, "file")
				writeFile(t, path, "")

				return path
			},

			assertErr:         errassert.Contains("not a directory"),
			wantConfigDir:     func(string) string { return "" },
			wantCertInfo:      require.Nil,
			wantDefaultRemote: "",
			wantRemotes:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			path := tc.setup(t, t.TempDir())
			cfg := &config.Config{}

			// Run test
			err := cfg.LoadConfig(path)

			// Assert
			tc.assertErr(t, err, "error of LoadConfig")
			require.Equal(t, tc.wantConfigDir(path), cfg.ConfigDir, "config dir")
			tc.wantCertInfo(t, cfg.CertInfo, "client certificate")
			require.Equal(t, tc.wantDefaultRemote, cfg.DefaultRemote, "default remote")
			require.Equal(t, tc.wantRemotes, cfg.Remotes, "remotes")
		})
	}
}

func TestConfig_LoadConfig_createsDirectories(t *testing.T) {
	t.Parallel()

	// Setup
	path := filepath.Join(t.TempDir(), "new")
	cfg := &config.Config{}

	// Run test
	err := cfg.LoadConfig(path)

	// Assert
	require.NoError(t, err, "load config into a new directory")
	require.DirExists(t, filepath.Join(path, "oidc-tokens"), "directory for OIDC tokens")
	require.FileExists(t, filepath.Join(path, "client.crt"), "client certificate")
	require.FileExists(t, filepath.Join(path, "client.key"), "client key")
}

func TestConfig_SaveConfig_missingConfigDir(t *testing.T) {
	t.Parallel()

	// Setup
	cfg := &config.Config{ConfigDir: filepath.Join(t.TempDir(), "missing")}

	// Run test
	err := cfg.SaveConfig()

	// Assert
	require.ErrorIs(t, err, os.ErrNotExist, "save into a missing config dir")
}

func TestConfig_SaveConfig_roundTrip(t *testing.T) {
	t.Parallel()

	// Setup
	dir := t.TempDir()
	saved := &config.Config{
		ConfigDir:     dir,
		DefaultRemote: "oidc",
		Remotes: map[string]config.Remote{
			"oidc": {Addr: "https://oidc:8443", AuthType: config.AuthTypeOIDC},
			"tls":  {Addr: "https://tls:8443", AuthType: config.AuthTypeTLS},
		},
	}

	// Run test
	err := saved.SaveConfig()
	require.NoError(t, err, "save config")

	loaded := &config.Config{}
	err = loaded.LoadConfig(dir)

	// Assert
	require.NoError(t, err, "load saved config")
	require.Equal(t, saved.DefaultRemote, loaded.DefaultRemote, "default remote")
	require.Equal(t, saved.Remotes, loaded.Remotes, "remotes")
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
