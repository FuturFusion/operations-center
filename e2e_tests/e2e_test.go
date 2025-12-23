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

	type testCase struct {
		name string

		testFunc func(t *testing.T, tmpDir string)

		skip bool
	}

	type setupType struct {
		name        string
		setupFunc   func(t *testing.T, tmpDir string)
		cleanupFunc func(t *testing.T) func()
	}

	setupTypeTests := []setupType{
		{
			name:        "with token",
			setupFunc:   setupIncusOSWithToken,
			cleanupFunc: cleanupIncusOS,
		},
		{
			name:        "with token seed",
			setupFunc:   setupIncusOSWithTokenSeed,
			cleanupFunc: cleanupIncusOS,
		},
	}

	testCases := []testCase{
		{
			name: "setup only",
			testFunc: func(t *testing.T, tmpDir string) {
				t.Helper()
			},
		},
		{
			name:     "create cluster",
			testFunc: createCluster,
		},
		{
			name:     "create cluster from cluster template",
			testFunc: createClusterFromTemplate,
		},
		{
			name:     "factory reset cluster",
			testFunc: factoryResetCluster,
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

	for _, ts := range setupTypeTests {
		t.Run(ts.name, func(t *testing.T) {
			setupOperationsCenter(t, tmpDir)

			if !noCleanup {
				t.Cleanup(ts.cleanupFunc(t))
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					if tc.skip {
						t.SkipNow()
					}

					stop := timeTrack(t, tc.name)
					defer stop()

					ts.setupFunc(t, tmpDir)

					tc.testFunc(t, tmpDir)
				})
			}
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
