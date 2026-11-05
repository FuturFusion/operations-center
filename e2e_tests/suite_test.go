package e2e

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/stretchr/testify/require"
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

	// prepare runs before the pool brings the servers into the required
	// state. Use it for changes, which a server only picks up during its
	// install, e.g. the content of its seed. The pool installs the servers of
	// the scenario again after prepare.
	prepare func(ctx context.Context, t *testing.T, tmpDir string)

	run func(ctx context.Context, t *testing.T, tmpDir string)
}

func (s e2eScenario) servers() []string {
	return append(slices.Clone(s.standalone), s.cluster...)
}

// poolSteps returns the steps, which the pool runs for the scenario.
func (s e2eScenario) poolSteps() []poolStep {
	var steps []poolStep
	if len(s.servers()) > 0 {
		steps = append(steps, poolStepStandalone)
	}

	if len(s.cluster) > 0 {
		steps = append(steps, poolStepCluster)
	}

	return steps
}

// scenarioStep is the step of a scenario, which runs or failed.
type scenarioStep string

const (
	// scenarioStepStart covers the checks before the setup, e.g. the time left
	// before the deadline of "go test".
	scenarioStepStart   scenarioStep = "start"
	scenarioStepSetup   scenarioStep = "setup"
	scenarioStepPrepare scenarioStep = "prepare"
	scenarioStepPool    scenarioStep = "pool"
	scenarioStepRun     scenarioStep = "run"
)

// scenarioResult is the result of a scenario for the summary of a suite.
type scenarioResult struct {
	name     string
	status   string
	duration time.Duration
	note     string
}

// pendingOCRestore is the state of Operations Center before the run of the
// last failed scenario. The next scenario or top level test restores it, since
// all of them share the Operations Center.
var pendingOCRestore *ocState

// runE2ESuite runs the scenarios in the given order as subtests of t. The
// scenarios share the setup of the Operations Center and the servers of pool.
// The setup runs in the first scenario selected by "-run" and again after a
// failed scenario. pool is nil, if no scenario requires a server.
//
// A failed scenario does not stop the suite, since the pool brings the servers
// into the state, which the next scenario requires. If a pool step fails
// repeatedly, the suite skips the remaining scenarios, which need this step
// (see poolStepFailureLimit).
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
	failures := poolStepFailures{}
	results := []scenarioResult{}

	for _, scenario := range scenarios {
		ran := false
		skipped := false
		blockedReason := ""
		step := scenarioStepStart
		start := time.Now()

		var stateBeforeRun *ocState

		ok := t.Run(scenario.name, func(t *testing.T) {
			ran = true

			defer func() {
				skipped = t.Skipped()
			}()

			blockedReason = failures.blockedReason(scenario.poolSteps())
			if blockedReason != "" {
				t.Skip(blockedReason)
			}

			runE2EScenario(t, scenario.name, func(ctx context.Context, t *testing.T, tmpDir string) {
				t.Helper()

				step = scenarioStepSetup

				if needsSetup {
					setupOperationsCenterOrMarkBroken(ctx, t, suite, tmpDir)

					if pendingOCRestore != nil {
						restoreOCStateOrMarkBroken(ctx, t, tmpDir, *pendingOCRestore)
						pendingOCRestore = nil
					}

					needsSetup = false
				}

				step = scenarioStepPrepare

				if scenario.prepare != nil {
					scenario.prepare(ctx, t, tmpDir)
					pool.markDirty(scenario.servers(), fmt.Sprintf("prepare of %s changed the install", scenario.name))
				}

				if len(scenario.servers()) > 0 {
					step = scenarioStepPool
				}

				if len(scenario.standalone) > 0 {
					pool.ensureStandalone(ctx, t, tmpDir, scenario.standalone)
				}

				if len(scenario.cluster) > 0 {
					pool.ensureCluster(ctx, t, tmpDir, scenario.cluster)
				}

				state := mustObserveOCState(t)
				stateBeforeRun = &state

				step = scenarioStepRun

				scenario.run(ctx, t, tmpDir)
			})
		})

		// The scenario is not selected by "-run" or "go test -failfast"
		// stopped the suite.
		if !ran {
			continue
		}

		result := scenarioResult{
			name:     scenario.name,
			status:   "PASS",
			duration: time.Since(start),
		}

		if skipped {
			result.status = "SKIP"
			result.note = cmp.Or(blockedReason, "Skipped, see the log of the scenario")
		}

		// The pool steps of a scenario, which reached its run, succeeded.
		if step == scenarioStepRun {
			failures.succeed(scenario.poolSteps())
		}

		// A failed scenario might leave the Operations Center and its servers
		// in a bad state.
		if !ok {
			failedStep := string(step)
			if step == scenarioStepPool {
				failedStep = string(pool.step)
				failures.fail(pool.step, scenario.name)
			}

			result.status = "FAIL"
			result.note = fmt.Sprintf("Failed in step %q", failedStep)

			needsSetup = true

			if stateBeforeRun != nil {
				pendingOCRestore = stateBeforeRun
			}

			if len(scenario.servers()) > 0 {
				pool.markDirty(scenario.servers(), fmt.Sprintf("scenario %s failed", scenario.name))
			}
		}

		if len(scenario.leavesDirty) > 0 {
			pool.markDirty(scenario.leavesDirty, fmt.Sprintf("scenario %s changed them", scenario.name))
		}

		results = append(results, result)
	}

	logSuiteSummary(t, results)
}

// logSuiteSummary logs the results of the scenarios, which ran.
func logSuiteSummary(t *testing.T, results []scenarioResult) {
	t.Helper()

	if len(results) == 0 {
		return
	}

	var summary strings.Builder

	w := tabwriter.NewWriter(&summary, 0, 0, 2, ' ', 0)
	for _, result := range results {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", result.status, result.name, result.duration.Round(time.Second), result.note)
	}

	_ = w.Flush()

	t.Logf("Summary of the suite %s:\n%s", t.Name(), summary.String())
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

	deadline, ok := t.Deadline()
	if ok {
		var err error
		timeout, err = cappedTestTimeout(time.Until(deadline), timeout, strechedTimeout(deadlineReserve), strechedTimeout(minTestDuration))
		require.NoError(t, err)
	}

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

// restoreOCStateOrMarkBroken restores the state of Operations Center, which a
// failed scenario left behind. A failure marks the setup as broken, since all
// the remaining tests share the Operations Center.
func restoreOCStateOrMarkBroken(ctx context.Context, t *testing.T, tmpDir string, reference ocState) {
	t.Helper()

	defer markSetupBrokenOnFailure(t, "restore of the Operations Center state")

	mustRestoreOCState(ctx, t, tmpDir, reference)
}

// mustRestoreOCState applies the changed system configs again and removes the
// added objects. A system config, which it can not restore, fails the test,
// since it changes the behavior of Operations Center. The remaining
// differences are only logged, since the tests select objects by name.
func mustRestoreOCState(ctx context.Context, t *testing.T, tmpDir string, reference ocState) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	diff := diffOCState(reference, mustObserveOCState(t))
	if diff.isEmpty() {
		return
	}

	t.Logf("The failed scenario left changes in Operations Center, restoring:\n%s", diff)

	for _, name := range diff.configs {
		filename := filepath.Join(tmpDir, fmt.Sprintf("system_%s_restore.json", name))
		err := os.WriteFile(filename, []byte(reference.configs[name]), 0o600)
		require.NoError(t, err)

		if name == "security" {
			err = applySystemSecurityWithContext(ctx, t, filename)
			require.NoError(t, err, "Failed to restore the security config")

			continue
		}

		mustRun(t, `../bin/operations-center.linux.%s system %s edit < %s`, cpuArch, name, filename)
	}

	// Restoring the ACME config might change the certificate, which
	// Operations Center serves.
	if slices.Contains(diff.configs, "security") && operationsCenterRemoteNeedsRetrust(ctx, t) {
		err := retrustOperationsCenterRemoteWithContext(ctx, t)
		require.NoError(t, err, "Failed to add the remote of the CLI again")
	}

	for _, kind := range ocObjectKinds {
		for _, id := range diff.added[kind.name] {
			err := fmtRunErr(runWithContext(ctx, t, kind.remove, cpuArch, id))
			if err != nil {
				t.Logf("Failed to remove the %s %q: %v", kind.name, id, err)
			}
		}
	}

	diff = diffOCState(reference, mustObserveOCState(t))
	require.Empty(t, diff.configs, "expect the system configs to be restored")

	if !diff.isEmpty() {
		t.Logf("Operations Center still differs from the state before the failed scenario:\n%s", diff)
	}
}

func skipUnlessE2ETestEnabled(t *testing.T) {
	t.Helper()

	e2eTest := os.Getenv("OPERATIONS_CENTER_E2E_TEST")
	runE2ETest, _ := strconv.ParseBool(e2eTest)
	if !runE2ETest {
		t.Skip("OPERATIONS_CENTER_E2E_TEST env var not set, skipping end 2 end tests.")
	}
}
