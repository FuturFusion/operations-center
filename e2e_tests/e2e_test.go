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

	tests := []struct {
		name string

		testFunc func(t *testing.T, tmpDir string)

		skip bool
	}{
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

	t.Logf("temporary directory: %s", tmpDir)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.SkipNow()
			}

			stop := timeTrack(t, tc.name)
			defer stop()

			setup(t, tmpDir)

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
