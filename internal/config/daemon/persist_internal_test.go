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
