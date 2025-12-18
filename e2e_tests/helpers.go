package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// isFile checks if a path is a regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	}

	if info.IsDir() || !info.Mode().Type().IsRegular() {
		return false
	}

	return true
}

// isExecutable checks if path is an executable file that the current user can run.
func isExecutable(t *testing.T, path string) bool {
	t.Helper()

	info, err := os.Stat(path)
	require.NoErrorf(t, err, "file %q", path)

	// Check if it's a regular file.
	if !info.Mode().IsRegular() {
		return false
	}

	// Check if it has execute permission.
	mode := info.Mode()
	if mode&0o111 == 0 {
		return false // No execute bit set
	}

	// Check if current user can actually execute it.
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)

	uid := uint32(os.Geteuid())
	gid := uint32(os.Getegid())

	// Owner execute permission.
	if stat.Uid == uid && mode&0o100 != 0 {
		return true
	}

	// Group execute permission.
	if stat.Gid == gid && mode&0o010 != 0 {
		return true
	}

	// Others execute permission.
	if mode&0o001 != 0 {
		return true
	}

	// Check supplementary groups.
	groups, _ := os.Getgroups()
	for _, g := range groups {
		if uint32(g) == stat.Gid && mode&0o010 != 0 {
			return true
		}
	}

	return false
}

type cmdResponse struct {
	command  string
	output   bytes.Buffer
	exitCode int
}

func (c cmdResponse) Output() string {
	return c.output.String()
}

func (c cmdResponse) Success() bool {
	return c.exitCode == 0
}

// mustRun executes the provided command in a shell.
// If running the command returns an error or if the command
// has a non 0 exit code, the test is failed.
func mustRun(t *testing.T, command string, args ...any) cmdResponse {
	t.Helper()

	resp, err := runWithContext(t.Context(), t, command, args...)
	require.NoError(t, err)
	if !resp.Success() {
		t.Fatalf("Run: %q failed with:\n%s", resp.command, resp.Output())
	}

	return resp
}

// mustRunWithTimeout is mustRun with an additional timeout.
// see mustRun for details.
func mustRunWithTimeout(t *testing.T, command string, timeout time.Duration, args ...any) cmdResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(timeout))
	defer cancel()

	resp, err := runWithContext(ctx, t, command, args...)
	require.NoError(t, err)
	if !resp.Success() {
		t.Fatalf("Run: %q failed with:\n%s", resp.command, resp.Output())
	}

	return resp
}

// mustRunWithContext is mustRun with a separate context.
// see mustRun for details.
func mustRunWithContext(ctx context.Context, t *testing.T, command string, args ...any) cmdResponse { //nolint:unparam
	t.Helper()

	resp, err := runWithContext(ctx, t, command, args...)
	require.NoError(t, err)
	if !resp.Success() {
		t.Fatalf("Run: %q failed with:\n%s", resp.command, resp.Output())
	}

	return resp
}

// run executes the provided command in a shell.
func run(t *testing.T, command string, args ...any) (cmdResponse, error) {
	t.Helper()

	return runWithContext(t.Context(), t, command, args...)
}

// runWithTimout executes the provided command in a shell and fails if not
// completed before the given timeout.
func runWithTimeout(t *testing.T, command string, timeout time.Duration, args ...any) (cmdResponse, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(timeout))
	defer cancel()

	return runWithContext(ctx, t, command, args...)
}

// runWithContext executes the provided command in a shell and accepts additionally
// a context.
func runWithContext(ctx context.Context, t *testing.T, command string, args ...any) (cmdResponse, error) {
	t.Helper()

	command = fmt.Sprintf(`bash -c %q`, command)
	command = fmt.Sprintf(command, args...)
	shellArgs, err := shellwords.Parse(command)
	if err != nil {
		return cmdResponse{}, fmt.Errorf("run parse command %q: %w", command, err)
	}

	name := shellArgs[0]
	if len(shellArgs) > 1 {
		shellArgs = shellArgs[1:]
	}

	resp := cmdResponse{
		command: fmt.Sprintf("%s %s", name, strings.Join(shellArgs, " ")),
	}

	cmd := exec.CommandContext(ctx, name, shellArgs...)
	cmd.Stdout = &resp.output
	cmd.Stderr = &resp.output

	err = cmd.Run()
	if err != nil {
		exitErr := &exec.ExitError{}
		if !errors.As(err, &exitErr) {
			debugf("command: %q\nerr: %v\noutput:\n%s", resp.command, err, resp.Output())
			return cmdResponse{}, fmt.Errorf("run: %q: %w", resp.command, err)
		}

		resp.exitCode = exitErr.ExitCode()
	}

	debugf("command: %q\nexit code: %d\noutput:\n%s", resp.command, resp.exitCode, resp.Output())

	return resp, nil
}

// mustWaitAgentRunningContext waits for the incus agent to be running inside
// the given VM.
func waitForSuccessWithTimeout(t *testing.T, desc string, command string, timeout time.Duration, args ...any) (success bool, err error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(timeout))
	defer cancel()

	count := 0
	for {
		resp, err := run(t, command, args...)
		if err != nil {
			return false, err
		}

		if resp.Success() {
			break
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds for %q", count, desc)
		}

		count++

		select {
		case <-ctx.Done():
			return false, nil

		case <-time.After(1 * time.Second):
		}
	}

	t.Logf("Success %q after %ds", desc, count)

	return true, nil
}

// mustWaitAgentRunning waits for the incus agent to be running inside the
// given VM. The test is failed on error.
func mustWaitAgentRunning(t *testing.T, vm string, args ...any) {
	t.Helper()

	mustWaitAgentRunningWithContext(t.Context(), t, vm, args...)
}

// mustWaitAgentRunningWithTimeout is the same as mustWaitAgentRunning with
// an additional timeout.
func mustWaitAgentRunningWithTimeout(t *testing.T, vm string, timeout time.Duration, args ...any) {
	t.Helper()

	timeoutCtx, cancel := context.WithTimeout(t.Context(), strechedTimeout(timeout))
	defer cancel()

	mustWaitAgentRunningWithContext(timeoutCtx, t, vm, args...)
}

// mustWaitAgentRunningWithContext is the same as mustWaitAgentRunning but
// additionally accepts a context.
func mustWaitAgentRunningWithContext(ctx context.Context, t *testing.T, vm string, args ...any) {
	t.Helper()

	err := waitAgentRunningWithContext(ctx, t, vm, args...)
	require.NoError(t, err)
}

// mustWaitAgentRunningContext waits for the incus agent to be running inside
// the given VM.
func waitAgentRunningWithContext(ctx context.Context, t *testing.T, vm string, args ...any) error {
	t.Helper()

	vm = fmt.Sprintf(vm, args...)

	count := 0
	for {
		resp, err := run(t, `incus exec %s true`, vm)
		if err != nil {
			return err
		}

		if resp.Success() {
			break
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds for agent on %s", count, vm)
		}

		count++

		select {
		case <-ctx.Done():
			return fmt.Errorf("Context done: %v", t.Context().Err())

		case <-time.After(1 * time.Second):
		}
	}

	t.Logf("Agent running on %s after %ds", vm, count)

	return nil
}

// mustWaitExpectedLog waits for the wanted content to appear in the logs
// of the unit in the vm. The test is failed on error.
func mustWaitExpectedLog(t *testing.T, vm string, unit string, want string, args ...any) {
	t.Helper()

	mustWaitExpectedLogWithContext(t.Context(), t, vm, unit, want, false, args...)
}

// mustWaitExpectedLogWithTimeout is the same as mustWaitExpectedLog but
// accepts an additional timeout.
func mustWaitExpectedLogWithTimeout(t *testing.T, vm string, unit string, want string, timeout time.Duration, args ...any) {
	t.Helper()

	timeoutCtx, cancel := context.WithTimeout(t.Context(), strechedTimeout(timeout))
	defer cancel()

	mustWaitExpectedLogWithContext(timeoutCtx, t, vm, unit, want, false, args...)
}

// mustWaitExpectedLogWithTimeout is the same as mustWaitExpectedLog but
// additionally accepts an context.
func mustWaitExpectedLogWithContext(ctx context.Context, t *testing.T, vm string, unit string, want string, isRegex bool, args ...any) {
	t.Helper()

	err := waitExpectedLogWithContext(ctx, t, vm, unit, want, isRegex, args...)
	require.NoError(t, err)
}

// waitExpectedLogWithContext waits for the wanted content to appear in the logs
// of the unit in the vm.
func waitExpectedLogWithContext(ctx context.Context, t *testing.T, vm string, unit string, want string, isRegex bool, args ...any) error {
	t.Helper()

	vm = fmt.Sprintf(vm, args...)

	count := 0
	for {
		resp, err := run(t, `incus exec %s -- bash -c "journalctl -b -u %s"`, vm, unit)
		if err != nil {
			return err
		}

		if isRegex {
			if regexp.MustCompile(want).MatchString(resp.Output()) {
				break
			}
		} else {
			if strings.Contains(resp.Output(), want) {
				break
			}
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds for log %q on %s", count, want, vm)
		}

		count++

		select {
		case <-ctx.Done():
			return fmt.Errorf("Context done: %v", t.Context().Err())

		case <-time.After(1 * time.Second):
		}
	}

	t.Logf("Log %q appeared on %s after %ds", want, vm, count)

	return nil
}

// mustWaitUpdatesReady waits for at least 1 update to be ready in Operations
// Center. The test is failed on error.
func mustWaitUpdatesReady(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(5*time.Minute))
	defer cancel()

	count := 0
	for {
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.update_status == "ready") | true ] | length > 0'`, cpuArch)
		foundReady, _ := strconv.ParseBool(strings.TrimSpace(resp.Output()))
		if foundReady {
			break
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds on updates in Operations Center", count)
		}

		count++

		select {
		case <-ctx.Done():
			t.Fatalf("Context done: %v", ctx.Err())

		case <-time.After(1 * time.Second):
		}
	}

	t.Logf("Updates present Operations Center after %ds", count)
}

func mustWaitOperationsCenterAPIReady(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(60*time.Second))
	defer cancel()

	count := 0
	for {
		resp, err := run(t, `../bin/operations-center.linux.%s provisioning server list`, cpuArch)
		require.NoError(t, err)
		if resp.Success() {
			break
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds on Operations Center API to become ready", count)
		}

		count++

		select {
		case <-ctx.Done():
			t.Fatalf("Context done: %v", ctx.Err())

		case <-time.After(1 * time.Second):
		}
	}

	t.Logf("Operations Center API ready after %ds", count)
}

func mustWaitIncusOSReady(t *testing.T, names []string) {
	t.Helper()

	timeoutCtx, cancel := context.WithTimeout(t.Context(), strechedTimeout(10*time.Minute))
	defer cancel()

	errgrp, errgrpctx := errgroup.WithContext(timeoutCtx)
	ok, _ := strconv.ParseBool(concurrentSetup)
	if !ok {
		errgrp.SetLimit(1)
	}

	for _, name := range names {
		errgrp.Go(func() (err error) {
			stop := timeTrack(t, fmt.Sprintf("mustWaitIncusOSReady %s", name), "false")
			defer stop()

			defer func() {
				if err != nil {
					err = fmt.Errorf("%s: %w", name, err)
				}
			}()

			t.Logf("Waiting for %s to be ready", name)
			err = waitAgentRunningWithContext(errgrpctx, t, name)
			if err != nil {
				return err
			}

			err = waitExpectedLogWithContext(errgrpctx, t, name, "incus-osd", "System is ready", false)
			if err != nil {
				return err
			}

			return nil
		})
	}

	err := errgrp.Wait()
	require.NoError(t, err, "Failed to create IncusOS VMs for e2e test")
}

func mustWaitInventoryReady(t *testing.T, names []string) {
	t.Helper()

	timeoutCtx, cancel := context.WithTimeout(t.Context(), strechedTimeout(3*time.Minute))
	defer cancel()

	errgrp, errgrpctx := errgroup.WithContext(timeoutCtx)
	ok, _ := strconv.ParseBool(concurrentSetup)
	if !ok {
		errgrp.SetLimit(1)
	}

	for _, name := range names {
		errgrp.Go(func() (err error) {
			stop := timeTrack(t, fmt.Sprintf("mustWaitInventoryReady %s", name), "false")
			defer stop()

			defer func() {
				if err != nil {
					err = fmt.Errorf("%s: %w", name, err)
				}
			}()

			t.Logf("Waiting for %s to be registered as ready in inventory", name)

			count := 0
			for {
				resp, err := run(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.name == "%s" and .server_status == "ready") ] | length == 1'`, cpuArch, name)
				if err != nil {
					return err
				}

				if resp.Success() {
					break
				}

				if count%10 == 0 {
					t.Logf("Waiting %ds for %s to be registered as ready in inventory", count, name)
				}

				count++

				select {
				case <-errgrpctx.Done():
					return fmt.Errorf("Context done: %w", t.Context().Err())

				case <-time.After(1 * time.Second):
				}
			}

			t.Logf("%s registered as ready in inventory after %ds", name, count)

			return nil
		})
	}

	err := errgrp.Wait()
	require.NoError(t, err, "Failed to create IncusOS VMs for e2e test")
}

// fmtRunErr takes the cmdResponse and the error of a run function
// and formats the error, on none 0 exit code.
func fmtRunErr(resp cmdResponse, err error) error {
	if err != nil {
		return err
	}

	if resp.exitCode != 0 {
		return fmt.Errorf("exit code %d:\n%s", resp.exitCode, resp.Output())
	}

	return nil
}

func mustNotBeAlreadyClustered(t *testing.T) {
	t.Helper()

	clusterListResp, err := run(t, "incus exec IncusOS01 -- incus cluster list")
	require.NoError(t, err)
	require.NotEqual(t, 0, clusterListResp.exitCode, "IncusOS01 is already part of a cluster")
}

func mustGetInstanceIPAndNames(t *testing.T, names []string) (instanceIPs []string, instanceNames []string) {
	t.Helper()

	instanceIPs = make([]string, 0, len(names))
	instanceNames = make([]string, 0, len(names))
	for _, name := range names {
		ipResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "%s") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | [ .address ] | first'`, name)
		instanceIPs = append(instanceIPs, strings.TrimSpace(ipResp.Output()))

		nameResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "%s") | .state.os_info.hostname'`, name)
		instanceNames = append(instanceNames, strings.TrimSpace(nameResp.Output()))
	}

	return instanceIPs, instanceNames
}

// indent indents the given input line by line by prefix.
func indent(in string, prefix string) string {
	buf := strings.Builder{}

	for line := range strings.Lines(in) {
		buf.WriteString(prefix)
		buf.WriteString(line)
	}

	return buf.String()
}

var (
	indentLevel   int
	indentLevelMu sync.Mutex
)

// timeTrack measures the time elapsed from its call until the returned
// stop function is called.
// The first optional argument is the override value for the function name
// (default function name of the caller).
// The second optional argument indicates, if the indentation should be
// increased (default: "true").
func timeTrack(t *testing.T, optionals ...string) (stop func()) {
	t.Helper()

	var name string
	if len(optionals) > 0 {
		name = optionals[0]
	} else {
		pc, _, _, _ := runtime.Caller(1)
		funcName := runtime.FuncForPC(pc).Name()
		name = funcName[strings.LastIndex(funcName, ".")+1:]
	}

	indent := 1
	if len(optionals) > 1 {
		b, _ := strconv.ParseBool(optionals[1])
		if !b {
			indent = 0
		}
	}

	indentLevelMu.Lock()
	defer indentLevelMu.Unlock()

	t.Logf(">%s Start: %s", strings.Repeat(">", indentLevel*2), name)
	start := time.Now()

	indentLevel += indent

	return func() {
		indentLevelMu.Lock()
		defer indentLevelMu.Unlock()

		indentLevel -= indent

		t.Logf("<%s Stop  : %s 🕛 %v", strings.Repeat("<", indentLevel*2), name, time.Since(start))
	}
}

// strechedTimeout returns the provided timeout multiplied by the global
// stretch factor. The global stretch factor can be configured by the
// OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR env var.
func strechedTimeout(timeout time.Duration) time.Duration {
	return time.Duration(float64(timeout) * timeoutStretchFactor)
}

// debugf prints debug messages to stdout, if the global debug variable is true.
// This can be configured by the
// OPERATIONS_CENTER_E2E_TEST_DEBUG env var.
func debugf(format string, args ...any) {
	if !debug {
		return
	}

	fmt.Println(indent(fmt.Sprintf(format, args...), "debug: "))
}
