package e2e

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2E_WithToken_SetupOnly(t *testing.T) {
	runE2ETest(
		t,
		"token - basic operations center interactions",
		func(t *testing.T, tmpDir string) {
			t.Helper()
		},
		basicOperationsCenterInteractions,
	)
}

func TestE2E_WithToken_RegisterServer(t *testing.T) {
	runE2ETest(
		t,
		"token - basic operations center interactions",
		setupIncusOSWithToken([]string{"IncusOS01"}),
		registerServer,
	)
}

func TestE2E_UpdatesCleanupAndRefresh(t *testing.T) {
	runE2ETest(
		t,
		"updates cleanup and refresh",
		func(t *testing.T, tmpDir string) {
			t.Helper()
		},
		basicOperationsCenterInteractionsUpdatesCleanupAndRefresh,
	)
}

func TestE2E_WithToken_CreateCluster(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		createCluster([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
	)
}

func TestE2E_WithToken_CreateSingleNodeCluster(t *testing.T) {
	runE2ETest(
		t,
		"token - create single node cluster",
		setupIncusOSWithToken([]string{"IncusOS01"}),
		createCluster([]string{"IncusOS01"}),
	)
}

func TestE2E_WithToken_CreateClusterAndAddServerAndRemoveServer(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03", "IncusOS04"}),
		createClusterAndAddServerAndRemoveServer(),
	)
}

func TestE2E_WithToken_CreateClusterFromClusterTemplate(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster from cluster template",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		createClusterFromTemplate,
	)
}

func TestE2E_WithToken_FactoryResetCluster(t *testing.T) {
	runE2ETest(
		t,
		"token - factory reset cluster",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		factoryResetCluster,
	)
}

func TestE2E_WithToken_FactoryResetClusterWithTokenSeed(t *testing.T) {
	runE2ETest(
		t,
		"token - factory reset cluster",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		factoryResetClusterWithTokenSeed,
	)
}

func TestE2E_WithTokenSeed_CreateCluster(t *testing.T) {
	runE2ETest(
		t,
		"token seed - create cluster",
		setupIncusOSWithTokenSeed,
		createCluster([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
	)
}

func TestE2E_WithTokenAndUpdateChannel_CreateAndUpdateCluster(t *testing.T) {
	runE2ETest(
		t,
		"token with update channel - create cluster and perform a rolling update",
		setupIncusOSWithTokenAndUpdateChannel,
		createClusterAndThenClusterUpdate,
	)
}

func TestE2E_FromManualUpload_CreateCluster(t *testing.T) {
	runE2ETest(
		t,
		"download update - upload update - create channel - assign manual uploaded update - create cluster",
		setupIncusOSFromManualUpload,
		createClusterWithChannelName("manual", []string{"IncusOS01", "IncusOS02", "IncusOS03"}),
	)
}

func runE2ETest(
	t *testing.T,
	name string,
	setup func(t *testing.T, tmpDir string),
	test func(t *testing.T, tmpDir string),
) {
	t.Helper()

	e2eTest := os.Getenv("OPERATIONS_CENTER_E2E_TEST")
	runE2ETest, _ := strconv.ParseBool(e2eTest)
	if !runE2ETest {
		t.Skip("OPERATIONS_CENTER_E2E_TEST env var not set, skipping end 2 end tests.")
	}

	tmpDir := setupE2ETest(t)

	debugOutput = &bytes.Buffer{}
	t.Cleanup(onTestFailDebugOutput(t, tmpDir))

	stop := timeTrack(t, name)
	defer stop()

	setup(t, tmpDir)

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
