package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestE2E_OperationsCenter(t *testing.T) {
	runE2ESuite(t, nil, []e2eScenario{
		{
			name: "basic_interactions",
			run:  basicOperationsCenterInteractions,
		},
		{
			name: "provisioning_updates_channels_tokens",
			run:  provisioningUpdatesChannelsAndTokens,
		},
		{
			name: "oidc_authentication",
			run:  oidcAuthentication,
		},
		{
			name: "openfga_authorization",
			run:  openFGAAuthorization,
		},
		{
			// Deletes all the updates, so it runs last.
			name: "updates_cleanup_and_refresh",
			run:  basicOperationsCenterInteractionsUpdatesCleanupAndRefresh,
		},
	})
}

func TestE2E_StableServers(t *testing.T) {
	pool := newServerPool(t, []string{"IncusOS01"}, prepareStableTokenISO)

	runE2ESuite(t, pool, []e2eScenario{
		{
			name:       "register_server",
			standalone: []string{"IncusOS01"},
			run:        registerServer,
		},
		{
			name:       "bmc_power_off_and_on",
			standalone: []string{"IncusOS01"},
			run:        serverBMCPowerOffAndOn("IncusOS01"),
		},
		{
			// Leaves IncusOS01 clustered, so it runs last.
			name:       "images_remote_launch_instance",
			standalone: []string{"IncusOS01"},
			run:        ocIncusImagesRemoteLaunchInstance([]string{"IncusOS01"}),
		},
	})
}

func TestE2E_WithToken_ACMERenewal(t *testing.T) {
	runE2ETest(
		t,
		"token - ACME renewal with private CA",
		func(ctx context.Context, t *testing.T, tmpDir string) {
			t.Helper()
		},
		acmeRenewal([]string{"IncusOS01", "IncusOS02"}),
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

func TestE2E_WithToken_CreateClusterAndAddServerAndRemoveServer(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03", "IncusOS04"}),
		createClusterAndAddServerAndRemoveServer(),
	)
}

func TestE2E_WithToken_CreateClusterAndForceRemoveLostServer(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		createClusterAndForceRemoveLostServer(),
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
		setupIncusOSWithTokenAndUpdateChannel([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		createClusterAndThenClusterUpdate,
	)
}

func TestE2E_WithTokenAndUpdateChannel_UpdateServerApplication(t *testing.T) {
	runE2ETest(
		t,
		"token with update channel - update a single application of a server",
		setupIncusOSWithTokenAndUpdateChannel([]string{"IncusOS01"}),
		updateServerApplication("IncusOS01"),
	)
}

func TestE2E_WithTokenAndUpdateChannel_UpdateServerOSOnly(t *testing.T) {
	runE2ETest(
		t,
		"token with update channel - update only the OS of a server",
		setupIncusOSWithTokenAndUpdateChannel([]string{"IncusOS01"}),
		updateServerOSOnly("IncusOS01"),
	)
}

func TestE2E_WithToken_CreateClusterAndRebootCluster(t *testing.T) {
	runE2ETest(
		t,
		"token - create cluster and perform an on demand rolling reboot",
		setupIncusOSWithToken([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
		createClusterAndThenClusterReboot,
	)
}

func TestE2E_WithTokenSeed_DeployServer(t *testing.T) {
	runE2ETest(
		t,
		"token seed - pre-register a server and deploy it via BMC",
		setupEmptyInstance("IncusOS01"),
		serverDeployment("IncusOS01"),
	)
}

func TestE2E_FromManualUpload_CreateCluster(t *testing.T) {
	runE2ETest(
		t,
		"download update - upload update - create channel - assign manual uploaded update - create cluster",
		setupIncusOSFromManualUpload,
		createClusterFromManualUpload([]string{"IncusOS01", "IncusOS02", "IncusOS03"}),
	)
}

func runE2ETest(
	t *testing.T,
	name string,
	setup func(ctx context.Context, t *testing.T, tmpDir string),
	test func(ctx context.Context, t *testing.T, tmpDir string),
) {
	t.Helper()

	if listOnly {
		t.Skip("OPERATIONS_CENTER_E2E_TEST_LIST env var set, only listing end 2 end tests.")
	}

	skipUnlessE2ETestEnabled(t)

	runE2EScenario(t, name, func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		setupOperationsCenterOrMarkBroken(ctx, t, t, tmpDir)

		setup(ctx, t, tmpDir)

		test(ctx, t, tmpDir)
	})
}

var (
	setupBrokenMu     sync.Mutex
	setupBrokenReport string
)

// setupOperationsCenterOrMarkBroken sets the Operations Center up and records a
// failure to do so, so the remaining tests skip instead of reproducing it.
func setupOperationsCenterOrMarkBroken(ctx context.Context, t *testing.T, owner *testing.T, tmpDir string) {
	t.Helper()

	// A failure unwinds through runtime.Goexit, which runs the deferred
	// functions, so the report is recorded either way.
	defer func() {
		if !t.Failed() {
			return
		}

		setupBrokenMu.Lock()
		defer setupBrokenMu.Unlock()

		if setupBrokenReport == "" {
			setupBrokenReport = fmt.Sprintf("setup of the Operations Center failed in %s", t.Name())
		}
	}()

	setupOperationsCenter(ctx, t, owner, tmpDir)
}

// setupBrokenReason returns the report of the test, which found the setup of the
// Operations Center broken, or an empty string, if none did so far.
func setupBrokenReason() string {
	setupBrokenMu.Lock()
	defer setupBrokenMu.Unlock()

	return setupBrokenReport
}

// e2eTestTmpDir verifies the preconditions of an end 2 end test and returns the
// temporary directory it works in.
func e2eTestTmpDir(t *testing.T) string {
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

	return tmpDir
}
