package e2e

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2E_WithToken_SetupOnly(t *testing.T) {
	runE2ETest(
		t,
		"token - setup only",
		setupIncusOSWithToken,
		cleanupIncusOS,
		func(t *testing.T, tmpDir string) {
			t.Helper()
			// Setup only
		},
		func(t *testing.T) func() {
			t.Helper()
			// Setup only, no cleanup
			return func() {}
		},
	)
}

func TestE2E_WithToken_CreateCluster(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster",
		setupIncusOSWithToken,
		cleanupIncusOS,
		createCluster,
		clusterCleanup,
	)
}

func TestE2E_WithToken_CreateClusterFromClusterTemplate(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster from cluster template",
		setupIncusOSWithToken,
		cleanupIncusOS,
		createClusterFromTemplate,
		clusterCleanup,
	)
}

func TestE2E_WithToken_FactoryResetCluster(t *testing.T) {
	runE2ETest(
		t,
		"token - factory reset cluster",
		setupIncusOSWithToken,
		cleanupIncusOS,
		factoryResetCluster,
		// TODO: more cleanup needed?
		clusterCleanup,
	)
}

func TestE2E_WithTokenSeed_CreateCluster(t *testing.T) {
	runE2ETest(
		t,
		"token seed - create cluster",
		setupIncusOSWithTokenSeed,
		cleanupIncusOS,
		createCluster,
		// TODO: Token seed cleanup needed?
		clusterCleanup,
	)
}

func runE2ETest(
	t *testing.T,
	name string,
	setup func(t *testing.T, tmpDir string),
	setupCleanup func(t *testing.T) func(),
	test func(t *testing.T, tmpDir string),
	testCleanup func(t *testing.T) func(),
) {
	t.Helper()

	e2eTest := os.Getenv("OPERATIONS_CENTER_E2E_TEST")
	runE2ETest, _ := strconv.ParseBool(e2eTest)
	if !runE2ETest {
		t.Skip("OPERATIONS_CENTER_E2E_TEST env var not set, skipping end 2 end tests.")
	}

	tmpDir := setupE2ETest(t)

	stop := timeTrack(t, name)
	defer stop()

	if !noCleanup {
		t.Cleanup(setupCleanup(t))
	}
	setup(t, tmpDir)

	if !noCleanup {
		t.Cleanup(testCleanup(t))
	}

	test(t, tmpDir)
}

func setupE2ETest(t *testing.T) string {
	t.Helper()

	// Precheck
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

	return tmpDir
}
