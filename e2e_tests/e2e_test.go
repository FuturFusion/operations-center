package e2e

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2E(t *testing.T) {
	e2eTest := os.Getenv("OPERATIONS_CENTER_E2E_TEST")
	runE2ETest, _ := strconv.ParseBool(e2eTest)
	if !runE2ETest {
		t.Skip("OPERATIONS_CENTER_E2E_TEST env var not set, skipping end 2 end tests.")
	}

	testCases := []struct {
		name string

		setupFunc   func(t *testing.T, tmpDir string)
		cleanupFunc func(t *testing.T) func()
		testFunc    func(t *testing.T, tmpDir string)
	}{
		{
			name:        "token - setup only",
			setupFunc:   setupIncusOSWithToken,
			cleanupFunc: cleanupIncusOS,
			testFunc: func(t *testing.T, tmpDir string) {
				t.Helper()
			},
		},
		{
			name:        "token - create cluster",
			setupFunc:   setupIncusOSWithToken,
			cleanupFunc: cleanupIncusOS,
			testFunc:    createCluster,
		},
		{
			name:        "token - create cluster from cluster template",
			setupFunc:   setupIncusOSWithToken,
			cleanupFunc: cleanupIncusOS,
			testFunc:    createClusterFromTemplate,
		},
		{
			name:        "token - factory reset cluster",
			setupFunc:   setupIncusOSWithToken,
			cleanupFunc: cleanupIncusOS,
			testFunc:    factoryResetCluster,
		},
		{
			name:        "token seed - create cluster",
			setupFunc:   setupIncusOSWithTokenSeed,
			cleanupFunc: cleanupIncusOS,
			testFunc:    createCluster,
		},
	}

	var err error

	preCheck(t)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	// We don't use the system /tmp, because we need to download large ISO files,
	// which might exceed the available space in /tmp.
	tmpDir := os.Getenv("OPERATIONS_CENTER_E2E_TEST_TMP_DIR")
	if tmpDir == "" {
		tmpDir, err = os.MkdirTemp(homeDir, "tmp-e2e-*")
	} else {
		err = os.MkdirAll(tmpDir, 0o700)
	}

	require.NoError(t, err)

	t.Logf("Temporary directory: %s", tmpDir)

	setupOperationsCenter(t, tmpDir)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stop := timeTrack(t, tc.name)
			defer stop()

			tc.setupFunc(t, tmpDir)
			if !noCleanup {
				t.Cleanup(tc.cleanupFunc(t))
			}

			tc.testFunc(t, tmpDir)
		})
	}
}

func preCheck(t *testing.T) {
	t.Helper()

	executables := []string{
		fmt.Sprintf("../bin/operations-center.linux.%s", cpuArch),
		"../bin/operations-centerd",
		"/usr/bin/incus",
	}

	for _, executable := range executables {
		if !isExecutable(t, executable) {
			t.Fatalf("%q is not executable by the current user", executable)
		}
	}
}
