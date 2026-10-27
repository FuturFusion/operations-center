package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
)

func Test_Init_persistsOnlyOnChange(t *testing.T) {
	dir := t.TempDir()
	env := &mock.EnvironmentMock{
		VarDirFunc:    func() string { return dir },
		IsIncusOSFunc: func() bool { return false },
	}

	previousStore := defaultStore.Load()
	previousInternalConfig := globalInternalConfig.Load()

	t.Cleanup(func() {
		defaultStore.Store(previousStore)
		globalInternalConfig.Store(previousInternalConfig)
	})

	filename := filepath.Join(dir, ConfigFilename)

	err := Init(env)
	require.NoError(t, err)

	fileInfo, err := os.Stat(filename)
	require.NoError(t, err)

	// The config holds credentials, so it must not be world readable.
	require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	before, err := os.ReadFile(filename)
	require.NoError(t, err)

	err = Init(env)
	require.NoError(t, err)

	fileInfoAfter, err := os.Stat(filename)
	require.NoError(t, err)
	require.Equal(t, fileInfo.ModTime(), fileInfoAfter.ModTime(), "config was rewritten although it did not change")

	after, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))

	// A config written before the daemon started restricting the permissions is
	// tightened even though its contents do not change.
	err = os.Chmod(filename, 0o644)
	require.NoError(t, err)

	err = Init(env)
	require.NoError(t, err)

	fileInfoTightened, err := os.Stat(filename)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fileInfoTightened.Mode().Perm())
}

func Test_persist_symlinkOutsideOfVarDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(env enver) error
	}{
		{
			name: "loadConfig",
			run: func(env enver) error {
				_, _, err := loadConfig(env)
				return err
			},
		},
		{
			name: "ensureConfigFileMode",
			run: func(env enver) error {
				return ensureConfigFileMode(env)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			parentDir := t.TempDir()
			outsideFile := filepath.Join(parentDir, "outside.yml")
			err := os.WriteFile(outsideFile, []byte("outside: true\n"), 0o644)
			require.NoError(t, err)

			varDir := filepath.Join(parentDir, "var")
			require.NoError(t, os.Mkdir(varDir, 0o700))
			require.NoError(t, os.Symlink(outsideFile, filepath.Join(varDir, ConfigFilename)))

			env := &mock.EnvironmentMock{
				VarDirFunc: func() string { return varDir },
			}

			// Run test
			err = tc.run(env)

			// Assert
			require.ErrorContains(t, err, "path escapes from parent", "the symlink must not be followed out of the var dir")

			fileInfo, err := os.Stat(outsideFile)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o644), fileInfo.Mode().Perm(), "the file outside of the var dir must not be changed")
		})
	}
}

func Test_saveToDisk_replacesSymlinkOutsideOfVarDir(t *testing.T) {
	t.Parallel()

	// Setup
	parentDir := t.TempDir()
	outsideFile := filepath.Join(parentDir, "outside.yml")
	err := os.WriteFile(outsideFile, []byte("outside: true\n"), 0o644)
	require.NoError(t, err)

	varDir := filepath.Join(parentDir, "var")
	require.NoError(t, os.Mkdir(varDir, 0o700))
	require.NoError(t, os.Symlink(outsideFile, filepath.Join(varDir, ConfigFilename)))

	env := &mock.EnvironmentMock{
		VarDirFunc: func() string { return varDir },
	}

	// Run test
	err = saveToDisk(env, config{})

	// Assert
	require.NoError(t, err)

	outside, err := os.ReadFile(outsideFile)
	require.NoError(t, err)
	require.Equal(t, "outside: true\n", string(outside), "the file outside of the var dir must not be written")

	fileInfo, err := os.Lstat(filepath.Join(varDir, ConfigFilename))
	require.NoError(t, err)
	require.True(t, fileInfo.Mode().IsRegular(), "the symlink is replaced by the config file")
}
