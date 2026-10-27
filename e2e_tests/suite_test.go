package e2e

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"
)

// e2eScenario is a single scenario of an end 2 end suite. It runs as a subtest
// of the suite, so it can be selected with "go test -run Suite/scenario".
type e2eScenario struct {
	// name is the name of the subtest. Use snake_case, since "go test"
	// replaces spaces with underscores.
	name string

	// standalone lists the servers of the pool, which the scenario requires
	// as ready servers, which are not clustered.
	standalone []string

	// cluster lists the servers of the pool, which the scenario requires as
	// the members of the cluster incus-os-cluster.
	cluster []string

	// leavesDirty lists the servers, which the pool installs again after the
	// scenario. Use it for changes, which the pool does not observe, e.g. the
	// version of a server.
	leavesDirty []string

	run func(ctx context.Context, t *testing.T, tmpDir string)
}

func (s e2eScenario) servers() []string {
	return append(slices.Clone(s.standalone), s.cluster...)
}

// runE2ESuite runs the scenarios in the given order as subtests of t. The
// scenarios share the setup of the Operations Center and the servers of pool.
// The setup runs in the first scenario selected by "-run" and again after a
// failed scenario. pool is nil, if no scenario requires a server.
func runE2ESuite(t *testing.T, pool *serverPool, scenarios []e2eScenario) {
	t.Helper()

	if listOnly {
		for _, scenario := range scenarios {
			t.Run(scenario.name, func(t *testing.T) {
				t.Skip("OPERATIONS_CENTER_E2E_TEST_LIST env var set, only listing end 2 end tests.")
			})
		}

		return
	}

	skipUnlessE2ETestEnabled(t)

	suite := t
	needsSetup := true

	for _, scenario := range scenarios {
		ok := t.Run(scenario.name, func(t *testing.T) {
			runE2EScenario(t, scenario.name, func(ctx context.Context, t *testing.T, tmpDir string) {
				t.Helper()

				if needsSetup {
					setupOperationsCenterOrMarkBroken(ctx, t, suite, tmpDir)
					needsSetup = false
				}

				if len(scenario.standalone) > 0 {
					pool.ensureStandalone(ctx, t, tmpDir, scenario.standalone)
				}

				if len(scenario.cluster) > 0 {
					pool.ensureCluster(ctx, t, tmpDir, scenario.cluster)
				}

				scenario.run(ctx, t, tmpDir)
			})
		})

		// A failed scenario might leave the Operations Center and its servers
		// in a bad state.
		if !ok {
			needsSetup = true

			if len(scenario.servers()) > 0 {
				pool.markDirty(scenario.servers())
			}
		}

		if len(scenario.leavesDirty) > 0 {
			pool.markDirty(scenario.leavesDirty)
		}
	}
}

// runE2EScenario runs a single end 2 end scenario with its own timeout, time
// tracking and debug output on failure.
func runE2EScenario(t *testing.T, name string, run func(ctx context.Context, t *testing.T, tmpDir string)) {
	t.Helper()

	logEnvSettings(t)

	reason := setupBrokenReason()
	if reason != "" {
		t.Skipf("Setup of the Operations Center is broken, skipping: %s", reason)
	}

	timeout := strechedTimeout(testTimeout)

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	defer func() {
		if ctx.Err() != nil {
			t.Errorf("Test exceeded its timeout of %v: %v", timeout, ctx.Err())
		}
	}()

	tmpDir := e2eTestTmpDir(t)

	resetDebugOutput()
	resetVMDebugInfo()

	debugOutputCollect := onTestFailDebugOutput(t, tmpDir)
	defer debugOutputCollect()

	stop := timeTrack(t, name)
	defer stop()

	run(ctx, t, tmpDir)
}

func skipUnlessE2ETestEnabled(t *testing.T) {
	t.Helper()

	e2eTest := os.Getenv("OPERATIONS_CENTER_E2E_TEST")
	runE2ETest, _ := strconv.ParseBool(e2eTest)
	if !runE2ETest {
		t.Skip("OPERATIONS_CENTER_E2E_TEST env var not set, skipping end 2 end tests.")
	}
}
