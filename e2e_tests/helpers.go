package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

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

const (
	// incusOSImageMinSize is a floor for the size of an IncusOS image.
	incusOSImageMinSize = 64 * 1024 * 1024

	// gptHeaderMagic marks the GPT header of an IncusOS image.
	gptHeaderMagic = "EFI PART"
)

// gptHeaderOffsets are the offsets, at which the GPT header is found. It lives
// in the second logical block, which puts it at 512 for a 512 byte and at 2048
// for a 4096 byte logical sector size. The IncusOS images use the latter.
var gptHeaderOffsets = []int64{512, 2048}

// errNotAnIncusOSImage returns an error, if the file at path is not a usable
// IncusOS image.
func errNotAnIncusOSImage(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("Failed to stat %q: %w", path, err)
	}

	if info.Size() < incusOSImageMinSize {
		return fmt.Errorf("File %q holds %d bytes, which is below the %d bytes expected of an IncusOS image", path, info.Size(), incusOSImageMinSize)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", path, err)
	}

	defer func() {
		_ = file.Close()
	}()

	magic := make([]byte, len(gptHeaderMagic))

	for _, offset := range gptHeaderOffsets {
		_, err := file.ReadAt(magic, offset)
		if err != nil {
			return fmt.Errorf("Failed to read %q at offset %d: %w", path, offset, err)
		}

		if string(magic) == gptHeaderMagic {
			return nil
		}
	}

	return fmt.Errorf("File %q carries no GPT header at any of the offsets %v, so it is not an IncusOS image", path, gptHeaderOffsets)
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
	output   *bytes.Buffer
	exitCode int
	err      error
	ctxErr   error
}

func (c cmdResponse) Output() string {
	return c.output.String()
}

func (c cmdResponse) OutputTrimmed() string {
	return strings.TrimSpace(c.output.String())
}

func (c cmdResponse) Success() bool {
	return c.err == nil && c.exitCode == 0
}

func (c cmdResponse) Error() string {
	return fmt.Sprintf("run %q produced %s\nOutput:\n%s\n", c.command, c.reason(), c.Output())
}

// reason describes, why the command failed. A command, which is killed, because
// its context is done, reports the exit code -1 and no error of its own, so the
// error of the context is the only hint about the actual reason.
func (c cmdResponse) reason() string {
	if c.ctxErr != nil {
		return fmt.Sprintf("exit code: %d and error: %v (killed, context done: %v)", c.exitCode, c.err, c.ctxErr)
	}

	return fmt.Sprintf("exit code: %d and error: %v", c.exitCode, c.err)
}

// mustRun executes the provided command in a shell.
// If running the command returns an error or if the command
// has a non 0 exit code, the test is failed.
func mustRun(t *testing.T, command string, args ...any) cmdResponse {
	t.Helper()

	resp := runWithContext(t.Context(), t, command, args...)
	require.NoError(t, resp.err)
	if !resp.Success() {
		t.Fatalf("Run: %q failed with:\n%s", resp.command, resp.Output())
	}

	return resp
}

// mustRunQuiet is mustRun for commands, which are executed in a poll loop.
// see mustRun and runQuietWithContext for details.
func mustRunQuiet(t *testing.T, command string, args ...any) cmdResponse {
	t.Helper()

	resp := runQuietWithContext(t.Context(), t, command, args...)
	require.NoError(t, resp.err)

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

	resp := runWithContext(ctx, t, command, args...)
	require.NoError(t, resp.err)
	if !resp.Success() {
		t.Fatalf("Run: %q failed with:\n%s", resp.command, resp.Output())
	}

	return resp
}

// mustRunWithContext is mustRun with a separate context.
// see mustRun for details.
func mustRunWithContext(ctx context.Context, t *testing.T, command string, args ...any) cmdResponse { //nolint:unparam
	t.Helper()

	resp := runWithContext(ctx, t, command, args...)
	require.NoError(t, resp.err)
	if !resp.Success() {
		t.Fatalf("Run: %q failed with:\n%s", resp.command, resp.Output())
	}

	return resp
}

// run executes the provided command in a shell.
func run(t *testing.T, command string, args ...any) cmdResponse {
	t.Helper()

	return runWithContext(t.Context(), t, command, args...)
}

// runWithTimout executes the provided command in a shell and fails if not
// completed before the given timeout.
func runWithTimeout(t *testing.T, command string, timeout time.Duration, args ...any) cmdResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(timeout))
	defer cancel()

	return runWithContext(ctx, t, command, args...)
}

// runWithContext executes the provided command in a shell and accepts additionally
// a context.
func runWithContext(ctx context.Context, t *testing.T, command string, args ...any) cmdResponse {
	t.Helper()

	return runCmdWithContext(ctx, t, false, command, args...)
}

// runQuietWithContext is runWithContext for commands, which are executed in a
// poll loop. Of a successful run only a summary is recorded in the debug
// output.
func runQuietWithContext(ctx context.Context, t *testing.T, command string, args ...any) cmdResponse {
	t.Helper()

	return runCmdWithContext(ctx, t, true, command, args...)
}

// runCmdWithContext executes the provided command in a shell. If quiet is true,
// the output of a successful run is not recorded in the debug output.
func runCmdWithContext(ctx context.Context, t *testing.T, quiet bool, command string, args ...any) cmdResponse {
	t.Helper()

	name := "bash"
	cmdArgs := []string{
		"-o", "pipefail", // fail the whole pipeline on error
		"-c",
		fmt.Sprintf(command, args...),
	}

	resp := cmdResponse{
		command: fmt.Sprintf("bash -o pipefail -c %q", fmt.Sprintf(command, args...)),
		output:  &bytes.Buffer{},
	}

	cmd := exec.CommandContext(ctx, name, cmdArgs...)

	e2eGoCoverDir := os.Getenv("OPERATIONS_CENTER_E2E_GOCOVERDIR")
	if e2eGoCoverDir != "" {
		env := os.Environ()
		env = append(env, "GOCOVERDIR="+e2eGoCoverDir)
		cmd.Env = env
	}

	cmd.Stdout = resp.output
	cmd.Stderr = resp.output

	err := cmd.Run()
	if err != nil {
		exitErr := &exec.ExitError{}
		if !errors.As(err, &exitErr) {
			debugf("command: %q\nerr: %v\noutput:\n%s", resp.command, err, resp.Output())

			resp.err = fmt.Errorf("run: %q: %w", resp.command, err)

			return resp
		}

		resp.exitCode = exitErr.ExitCode()
		resp.ctxErr = ctx.Err()
	}

	if quiet && resp.Success() {
		debugf("command: %q\nexit code: %d\noutput suppressed: %d bytes", resp.command, resp.exitCode, resp.output.Len())
	} else {
		debugf("command: %q\nexit code: %d\noutput:\n%s", resp.command, resp.exitCode, resp.Output())
	}

	return resp
}

// waitForSuccessWithTimout retries a command until it is executed successfully
// or the timeout is exceeded.
func waitForSuccessWithTimeout(ctx context.Context, t *testing.T, desc string, command string, timeout time.Duration, args ...any) (success bool, err error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	count := 0
	for {
		resp := runQuietWithContext(ctx, t, command, args...)
		if resp.err != nil {
			return false, resp.err
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

const (
	debugInfoLines        = 100
	debugInfoConsoleBytes = 32 * 1024
	hostDebugInfoKey      = "<host>"
)

var (
	debugInfoLoggedMu sync.Mutex
	debugInfoLogged   = map[string]bool{}
)

// debugInfoTodo returns the subset of vms, for which the debug information has
// not been collected yet, and reports, whether the host level debug information
// is still to be collected. Everything it returns is marked as collected.
func debugInfoTodo(vms []string) (todo []string, host bool) {
	debugInfoLoggedMu.Lock()
	defer debugInfoLoggedMu.Unlock()

	todo = make([]string, 0, len(vms))

	for _, vm := range vms {
		if debugInfoLogged[vm] {
			continue
		}

		debugInfoLogged[vm] = true

		todo = append(todo, vm)
	}

	host = !debugInfoLogged[hostDebugInfoKey]
	debugInfoLogged[hostDebugInfoKey] = true

	return todo, host
}

func resetVMDebugInfo() {
	debugInfoLoggedMu.Lock()
	defer debugInfoLoggedMu.Unlock()

	debugInfoLogged = map[string]bool{}
}

// logVMDebugInfo collects debug information for the given VMs and writes it to
// the test log. It is meant to be used on failure paths, where the error of the
// failing operation alone does not explain, what went wrong inside of the VM.
func logVMDebugInfo(t *testing.T, vms ...string) {
	t.Helper()

	vms, logHost := debugInfoTodo(vms)
	if len(vms) == 0 && !logHost {
		return
	}

	// Use detached contexts, since the context of the failing operation is
	// likely already cancelled at this stage.
	logCmd := func(what string, truncate func(string) string, command string, args ...any) {
		debugCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		resp := runQuietWithContext(debugCtx, t, command, args...)
		if resp.Success() {
			t.Logf("%s:\n%s", what, truncate(resp.Output()))

			return
		}

		t.Logf("failed to get %s: %s", what, resp.Error())
	}

	lines := func(s string) string {
		return tailMsg(s, debugInfoLines)
	}

	console := func(s string) string {
		return sanitizeConsoleLog(s, debugInfoConsoleBytes)
	}

	if logHost {
		logCmd("incus list", lines, "incus list")
		logCmd("storage pool", lines, "incus storage info default")
		logCmd("storage volumes", lines, "incus storage volume list default")
		logCmd("free disk space", lines, "df -h")
		logCmd("zpool list", lines, "zpool list")
	}

	for _, vm := range vms {
		logCmd(fmt.Sprintf("incus info for %q", vm), lines, "incus info %s --show-log", vm)
		logCmd(fmt.Sprintf("incus console log for %q", vm), console, "incus console %s --show-log", vm)
		logCmd(fmt.Sprintf("incus-osd log for %q", vm), lines, `incus exec %s -- bash -c "journalctl -b -u incus-osd --no-pager -n 100"`, vm)
		logCmd(fmt.Sprintf("incus-osd unit state for %q", vm), lines, `incus exec %s -- bash -c "systemctl status --no-pager incus-osd"`, vm)
	}
}

// mustWaitAgentRunning waits for the incus agent to be running inside the
// given VM. The test is failed on error.
func mustWaitAgentRunning(ctx context.Context, t *testing.T, vm string, args ...any) {
	t.Helper()

	err := waitAgentRunningWithContext(ctx, t, vm, args...)
	require.NoError(t, err)
}

// mustWaitAgentRunningWithTimeout is the same as mustWaitAgentRunning with
// an additional timeout.
func mustWaitAgentRunningWithTimeout(ctx context.Context, t *testing.T, vm string, timeout time.Duration, args ...any) {
	t.Helper()

	timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	mustWaitAgentRunning(timeoutCtx, t, vm, args...)
}

const (
	agentWaitAttemptTimeout     = 30 * time.Second
	incusOSStartupProbeInterval = 10 * time.Second
	incusOSStartupRestartGrace  = 2 * time.Minute
)

// waitAgentRunningWithContext waits for the incus agent to be running inside
// the given VM. It keeps waiting until the agent shows up or the context is
// done. If the instance is found not to be running, it is started again.
func waitAgentRunningWithContext(ctx context.Context, t *testing.T, vm string, args ...any) error {
	t.Helper()

	start := time.Now()

	vm = fmt.Sprintf(vm, args...)

	deadline, hasDeadline := ctx.Deadline()

	lastStatus := ""
	lastErr := ""

	nextStartupProbe := time.Now().Add(incusOSStartupProbeInterval)
	startupRestarted := false

	var startupErrSeen error

	// errGiveUp reports the given unrecoverable instance state, after dumping
	// the debug information. Once the instance is in such a state, waiting for
	// it or restarting it is pointless.
	errGiveUp := func(stateErr error) error {
		logVMDebugInfo(t, vm)

		return fmt.Errorf("Giving up waiting for the incus agent on %q after %s: %w", vm, time.Since(start).String(), stateErr)
	}

	// The instance might already be beyond rescue before the first wait.
	stateErr := errUnrecoverableInstanceState(ctx, t, vm)
	if stateErr != nil {
		return errGiveUp(stateErr)
	}

	for attempt := 0; ctx.Err() == nil; attempt++ {
		timeoutSeconds := -1 // -1 disables the timeout for incus wait.
		if hasDeadline {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				break
			}

			timeoutSeconds = max(int(min(remaining, agentWaitAttemptTimeout).Seconds()), 1)
		}

		resp := runQuietWithContext(ctx, t, `incus wait %s agent --timeout %d`, vm, timeoutSeconds)
		if resp.Success() {
			t.Logf("Agent running on %q after %s", vm, time.Since(start).String())

			return nil
		}

		lastErr = resp.Error()

		if ctx.Err() != nil {
			break
		}

		status, err := instanceStatusWithContext(ctx, t, vm)
		if err == nil {
			lastStatus = status
		}

		stateErr = errUnrecoverableStatus(vm, lastStatus)
		if stateErr != nil {
			return errGiveUp(stateErr)
		}

		if lastStatus != instanceStatusRunning {
			// The instance is not running, e.g. it shut down instead of
			// rebooting after the installation, so start it again before
			// waiting for the agent any further.
			t.Logf("Instance %s is in status %q, try restart", vm, lastStatus)

			startErr := startInstanceWithContext(ctx, t, vm)
			if startErr != nil {
				t.Logf(`failed to re-start incus: %v`, startErr)
			}
		} else {
			if attempt%10 == 0 {
				t.Logf("Waiting %s for agent on %s, instance status %q", time.Since(start).Truncate(time.Second), vm, lastStatus)
			}

			if time.Now().After(nextStartupProbe) {
				nextStartupProbe = time.Now().Add(incusOSStartupProbeInterval)

				startupErr := errIncusOSStartupFailure(ctx, t, vm)
				if startupErr != nil {
					startupErrSeen = startupErr

					if startupRestarted {
						return errGiveUp(startupErr)
					}

					startupRestarted = true

					t.Logf("Restarting %s once, since incus-osd failed to start: %v", vm, startupErr)

					restartErr := restartInstanceWithContext(ctx, t, vm)
					if restartErr != nil {
						t.Logf("failed to restart %s: %v", vm, restartErr)
					}

					nextStartupProbe = time.Now().Add(incusOSStartupRestartGrace)
				}
			}
		}

		select {
		case <-ctx.Done():

		case <-time.After(1 * time.Second):
		}
	}

	logVMDebugInfo(t, vm)

	if lastErr == "" {
		lastErr = fmt.Sprintf("context done: %v", ctx.Err())
	}

	if startupErrSeen != nil {
		return fmt.Errorf("Failed to wait for incus agent on %q after %s, last instance status %q, incus-osd failed to start on it earlier (%v): %s", vm, time.Since(start).String(), lastStatus, startupErrSeen, lastErr)
	}

	return fmt.Errorf("Failed to wait for incus agent on %q after %s, last instance status %q: %s", vm, time.Since(start).String(), lastStatus, lastErr)
}

// mustWaitExpectedLog waits for the wanted content to appear in the logs
// of the unit in the vm. The test is failed on error.
func mustWaitExpectedLog(ctx context.Context, t *testing.T, vm string, unit string, want string, args ...any) {
	t.Helper()

	mustWaitExpectedLogWithContext(ctx, t, vm, unit, want, false, args...)
}

// mustWaitExpectedLogWithTimeout is the same as mustWaitExpectedLog but
// accepts an additional timeout.
func mustWaitExpectedLogWithTimeout(ctx context.Context, t *testing.T, vm string, unit string, want string, timeout time.Duration, args ...any) {
	t.Helper()

	timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
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

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}

// tailMsg returns the last n lines of s, prefixed with a note, if lines were
// dropped.
func tailMsg(s string, n int) string {
	total := len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
	if total <= n {
		return tail(s, n)
	}

	return fmt.Sprintf("[truncated, showing the last %d of %d lines]\n%s", n, total, tail(s, n))
}

// ansiEscapeSequence matches the terminal escape sequences, which make up the
// bulk of a console log. The escape character is allowed to repeat, since it
// does so in the console log of an IncusOS VM. A leftover escape character,
// which does not introduce a sequence understood here, is dropped as well.
var ansiEscapeSequence = regexp.MustCompile(`\x1b+\[[0-?]*[ -/]*[@-~]|\x1b+[()][AB012]|\x1b+[=>]|\x1b+\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b`)

// sanitizeConsoleLog strips the terminal escape sequences from a console log
// and returns at most the last maxBytes bytes of the result, prefixed with a
// note, if content was dropped.
func sanitizeConsoleLog(in string, maxBytes int) string {
	out := ansiEscapeSequence.ReplaceAllString(in, "")
	if len(out) <= maxBytes {
		return out
	}

	truncated := out[len(out)-maxBytes:]

	// Do not cut a multi byte rune in half.
	for len(truncated) > 0 && !utf8.RuneStart(truncated[0]) {
		truncated = truncated[1:]
	}

	return fmt.Sprintf("[truncated, showing the last %d of %d bytes]\n%s", len(truncated), len(out), truncated)
}

// waitExpectedLogWithContext waits for the wanted content to appear in the logs
// of the unit in the vm.
func waitExpectedLogWithContext(ctx context.Context, t *testing.T, vm string, unit string, want string, isRegex bool, args ...any) error {
	t.Helper()

	vm = fmt.Sprintf(vm, args...)

	count := 0
	lastErr := ""
	lastLog := ""

	for {
		resp := runQuietWithContext(ctx, t, `incus exec %s -- bash -c "journalctl -b -u %s"`, vm, unit)
		if resp.err != nil {
			return resp.err
		}

		if resp.Success() {
			lastErr = ""
			lastLog = resp.Output()

			if isRegex {
				if regexp.MustCompile(want).MatchString(resp.Output()) {
					break
				}
			} else {
				if strings.Contains(resp.Output(), want) {
					break
				}
			}
		} else if ctx.Err() == nil {
			// Reading the log fails as long as the VM is not (yet) reachable,
			// e.g. while it is booting, so keep retrying, but remember the
			// reason in order to report it, if we run out of time.
			lastErr = resp.Error()

			if count%10 == 0 {
				t.Logf("Failed to read log of unit %q on %s: %s", unit, vm, lastErr)

				// The log of an instance, which is not running, is not
				// readable. If the instance can not recover, there is no point
				// in waiting for the remainder of the timeout.
				stateErr := errUnrecoverableInstanceState(ctx, t, vm)
				if stateErr == nil {
					// The instance stays in the status "Running", if incus-osd
					// aborted the boot, so the console log is the only source,
					// which reveals this.
					stateErr = errIncusOSStartupFailure(ctx, t, vm)
				}

				if stateErr != nil {
					logVMDebugInfo(t, vm)

					return fmt.Errorf("Giving up waiting for log %q on %s after %ds: %w", want, vm, count, stateErr)
				}
			}
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds for log %q on %s", count, want, vm)
		}

		count++

		select {
		case <-ctx.Done():
			if lastLog != "" {
				debugf("last log of unit %q on %s:\n%s", unit, vm, lastLog)
			}

			if lastErr != "" {
				return fmt.Errorf("Timed out after %ds waiting for log %q on %s, last error: %s: %w", count, want, vm, lastErr, ctx.Err())
			}

			// The log of the unit was readable, it just never contained what we
			// were waiting for, so report its tail, since it is the only hint
			// about what the unit was busy with instead.
			return fmt.Errorf("Timed out after %ds waiting for log %q on %s, last log of unit %q:\n%s\n: %w", count, want, vm, unit, tail(lastLog, 50), ctx.Err())

		case <-time.After(1 * time.Second):
		}
	}

	t.Logf("Log %q appeared on %s after %ds", want, vm, count)

	return nil
}

// mustWaitUpdatesReady waits for at least 1 update to be ready in Operations
// Center. The test is failed on error.
func mustWaitUpdatesReady(ctx context.Context, t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(15*time.Minute))
	defer cancel()

	count := 0
	for {
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.update_status == "ready") | true ] | length > 1'`, cpuArch)
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

	printUpdateList(t)
}

func mustWaitIncusOSReady(ctx context.Context, t *testing.T, names []string) {
	t.Helper()

	const (
		agentTimeout = 5 * time.Minute
		logTimeout   = 5 * time.Minute
	)

	timeout := agentTimeout + logTimeout
	if !concurrentSetup {
		timeout = time.Duration(int(timeout) * len(names))
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	errgrp, errgrpctx := errgroup.WithContext(timeoutCtx)
	if !concurrentSetup {
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

			agentWaitCtx, cancel := context.WithTimeout(errgrpctx, strechedTimeout(agentTimeout))
			err = waitAgentRunningWithContext(agentWaitCtx, t, name)
			cancel()

			if err != nil {
				return err
			}

			logWaitCtx, cancel := context.WithTimeout(errgrpctx, strechedTimeout(logTimeout))
			err = waitExpectedLogWithContext(logWaitCtx, t, name, "incus-osd", "System is ready", false)
			cancel()

			if err != nil {
				return err
			}

			return nil
		})
	}

	err := errgrp.Wait()
	if err != nil {
		logVMDebugInfo(t, names...)

		require.NoError(t, err, "Failed to wait for incus agents to become ready")
	}
}

func mustWaitInventoryReady(ctx context.Context, t *testing.T, names []string) {
	t.Helper()

	timeout := 3 * time.Minute
	if !concurrentSetup {
		timeout = time.Duration(int(timeout) * len(names))
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	errgrp, errgrpctx := errgroup.WithContext(timeoutCtx)
	if !concurrentSetup {
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
				resp := runWithContext(errgrpctx, t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.name == "%s" and .server_status == "ready") ] | length == 1'`, cpuArch, name)
				if resp.err != nil {
					return resp.err
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
					return fmt.Errorf("Timed out after %ds waiting for %s to be registered as ready in inventory: %w", count, name, errgrpctx.Err())

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

func waitForTCPPort(ctx context.Context, t *testing.T, hostPort string, interval time.Duration) error {
	t.Helper()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout reached while waiting for %s: %w", hostPort, ctx.Err())

		default:
			conn, err := net.DialTimeout("tcp", hostPort, interval)
			if err == nil {
				_ = conn.Close()

				return nil
			}

			time.Sleep(interval)
		}
	}
}

// e2eHostAddress returns the address of the e2e host on the network, the
// OperationsCenter VM is attached to. This is the address, at which services,
// which are served in-process by the tests, are reachable from inside the
// OperationsCenter VM.
func e2eHostAddress(t *testing.T) string {
	t.Helper()

	if hostAddress != "" {
		return hostAddress
	}

	networkResp := mustRun(t, `incus list -f json | jq -r -e '[ .[] | select(.name == "OperationsCenter") | .expanded_devices | to_entries[] | select(.value.type == "nic") | (.value.network // .value.parent // empty) ] | first'`)
	network := networkResp.OutputTrimmed()
	require.NotEmpty(t, network, "Failed to determine the network of the OperationsCenter VM")

	// For a managed bridge, the first address of ipv4.address is the address of
	// the host on that bridge.
	resp := run(t, `incus network get %s ipv4.address | cut -d / -f 1`, network)
	address := resp.OutputTrimmed()
	if resp.Success() && net.ParseIP(address) != nil {
		return address
	}

	// Fall back to the address configured on the host interface, which covers
	// unmanaged bridges and networks with an externally managed address.
	resp = mustRun(t, `ip -4 -json addr show dev %s | jq -r -e '.[0].addr_info[0].local'`, network)
	address = resp.OutputTrimmed()
	require.NotNilf(t, net.ParseIP(address), "Failed to determine the address of the host on network %q", network)

	return address
}

// fmtRunErr takes the cmdResponse and the error of a run function
// and formats the error, on none 0 exit code.
func fmtRunErr(resp cmdResponse) error {
	if resp.err != nil {
		return resp.err
	}

	if resp.exitCode != 0 {
		if resp.ctxErr != nil {
			return fmt.Errorf("exit code %d (killed, context done: %v):\nOutput:\n%s\n", resp.exitCode, resp.ctxErr, resp.Output())
		}

		return fmt.Errorf("exit code %d:\nOutput:\n%s\n", resp.exitCode, resp.Output())
	}

	return nil
}

func mustNotBeAlreadyClustered(t *testing.T) {
	t.Helper()

	clusterListResp := run(t, "incus exec IncusOS01 -- incus cluster list")
	require.NoError(t, clusterListResp.err)
	require.NotEqual(t, 0, clusterListResp.exitCode, "IncusOS01 is already part of a cluster")
}

func mustDeleteClusterImages(t *testing.T, clusterName string) {
	t.Helper()

	t.Log("Delete cached images from cluster")
	resp := mustRun(t, `incus image list %s: --all-projects -f json | jq -r '.[] | .project + " " + .fingerprint + " " + ((.locations // []) | join(","))'`, clusterName)

	for line := range strings.Lines(resp.OutputTrimmed()) {
		image := strings.Fields(line)
		if len(image) < 2 {
			continue
		}

		t.Logf("Delete image %s/%s", image[0], image[1])
		mustRun(t, `incus --project %s image delete %s:%s`, image[0], clusterName, image[1])
	}
}

// operationsCenterIPAddress returns the global IPv4 address of the
// OperationsCenter VM.
func operationsCenterIPAddress(t *testing.T) string {
	t.Helper()

	resp := mustRun(t, `incus list -f json | jq -r -e '[ .[] | select(.name == "OperationsCenter") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address ] | first'`)
	address := resp.OutputTrimmed()
	require.NotNilf(t, net.ParseIP(address), "Failed to determine the address of the OperationsCenter VM, got %q", address)

	return address
}

// mustDownloadAlpineImageFiles downloads the given files of the most recent
// alpine edge image for the CPU architecture under test into targetDir and
// returns the version identifier of the downloaded image.
//
// "rootfs.squashfs" is stored as "root.squashfs", which is the file name
// expected by Operations Center.
func mustDownloadAlpineImageFiles(t *testing.T, targetDir string, filenames ...string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	err := os.MkdirAll(targetDir, 0o700)
	require.NoError(t, err)

	resp := mustRunWithTimeout(t, `curl -sf "https://images.linuxcontainers.org/streams/v1/images.json" | jq -r -e '.products."alpine:edge:%s:default".versions | keys | last'`, time.Minute, cpuArch)
	version := resp.OutputTrimmed()
	require.NotEmpty(t, version, "Failed to determine the current alpine edge image version")

	for _, filename := range filenames {
		targetFilename := filename
		if filename == "rootfs.squashfs" {
			targetFilename = "root.squashfs"
		}

		mustRunWithTimeout(t, `curl -sfL "https://images.linuxcontainers.org/images/alpine/edge/%[1]s/default/%[2]s/%[3]s" -o %[4]s`, 5*time.Minute, cpuArch, version, filename, filepath.Join(targetDir, targetFilename))
	}

	return version
}

// mustSHA256 returns the hex encoded sha256 checksum of the given file.
func mustSHA256(t *testing.T, filename string) string {
	t.Helper()

	body, err := os.ReadFile(filename)
	require.NoErrorf(t, err, "Failed to read %q", filename)

	return fmt.Sprintf("%x", sha256.Sum256(body))
}

// mustWriteFileWithContent writes a file of the given size with deterministic
// content and returns its hex encoded sha256 checksum.
//
// The content is only used as payload of an image version, Operations Center
// does not interpret it.
func mustWriteFileWithContent(t *testing.T, filename string, size int) string {
	t.Helper()

	content := make([]byte, 0, size+sha256.Size)
	for i := 0; len(content) < size; i++ {
		sum := sha256.Sum256(fmt.Appendf(nil, "%s/%d", filepath.Base(filename), i))
		content = append(content, sum[:]...)
	}

	content = content[:size]

	err := os.WriteFile(filename, content, 0o600)
	require.NoErrorf(t, err, "Failed to write %q", filename)

	return fmt.Sprintf("%x", sha256.Sum256(content))
}

const (
	instanceStatusRunning = "Running"
	instanceStatusError   = "Error"
)

func instanceStatusWithContext(ctx context.Context, t *testing.T, name string) (string, error) {
	t.Helper()

	resp := runWithContext(ctx, t, `incus list -f json | jq -r '.[] | select(.name == "%s") | .status'`, name)

	err := fmtRunErr(resp)
	if err != nil {
		return "", fmt.Errorf("Failed to get status of instance %q: %w", name, err)
	}

	return resp.OutputTrimmed(), nil
}

// errUnrecoverableInstanceState returns an error, if the given instance is in a
// state, it can not recover from on its own. It returns nil, if the state can
// not be determined, since this is most likely a transient condition.
func errUnrecoverableInstanceState(ctx context.Context, t *testing.T, name string) error {
	t.Helper()

	status, err := instanceStatusWithContext(ctx, t, name)
	if err != nil {
		return nil
	}

	return errUnrecoverableStatus(name, status)
}

// errUnrecoverableStatus returns an error, if the given status is one, the
// instance can not recover from on its own.
func errUnrecoverableStatus(name string, status string) error {
	if status != instanceStatusError {
		return nil
	}

	return fmt.Errorf("Instance %[1]q is in status %[2]q and can not recover on its own. The cause is on the host, most likely an exhausted storage pool or filesystem, see the output of `incus info %[1]s --show-log` in the debug information below", name, status)
}

// incusOSStartupErrors are fragments, which appear on the console of an IncusOS
// VM, if incus-osd failed to start. The known instance of this is incus-osd
// aborting the boot with "unable to configure incus-agent", if restarting
// incus-agent.service fails.
//
// incus-osd exits in this case and IncusOS paints the error on the console,
// while the instance itself stays in the status "Running". Neither the instance
// status nor waiting any longer for the incus agent therefore reveals or
// resolves the situation.
var incusOSStartupErrors = []string{
	"incus-osd.service: Failed with result",
	"IncusOS critical startup error",
}

// errIncusOSStartupFailure returns an error, if incus-osd failed to start
// inside the given VM. It returns nil, if this can not be determined, since
// this is most likely a transient condition.
func errIncusOSStartupFailure(ctx context.Context, t *testing.T, name string) error {
	t.Helper()

	resp := runQuietWithContext(ctx, t, `incus console %s --show-log`, name)
	if !resp.Success() {
		return nil
	}

	fragment, found := incusOSStartupError(resp.Output())
	if !found {
		return nil
	}

	return fmt.Errorf("incus-osd failed to start on %[1]q, the console log contains %[2]q, see the output of `incus console %[1]s --show-log` in the debug information below", name, fragment)
}

// incusOSStartupError returns the fragment of incusOSStartupErrors found in the
// given console log, if any.
func incusOSStartupError(console string) (fragment string, found bool) {
	// The messages of systemd appear as plain text on the console, while the
	// error screen of IncusOS is drawn with escape sequences in between, so
	// match against both the raw and the sanitized console log.
	sanitized := ansiEscapeSequence.ReplaceAllString(console, "")

	for _, fragment := range incusOSStartupErrors {
		if strings.Contains(console, fragment) || strings.Contains(sanitized, fragment) {
			return fragment, true
		}
	}

	return "", false
}

func mustInstanceStatus(ctx context.Context, t *testing.T, name string) string {
	t.Helper()

	status, err := instanceStatusWithContext(ctx, t, name)
	require.NoError(t, err)

	return status
}

func waitInstanceStatusRunning(ctx context.Context, t *testing.T, name string, timeout time.Duration) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	count := 0
	lastStatus := ""

	for {
		status, err := instanceStatusWithContext(ctx, t, name)
		if err == nil {
			lastStatus = status

			if status == instanceStatusRunning {
				return status
			}
		}

		if count%10 == 0 {
			t.Logf("Waiting %ds for instance %q to become running, current status: %q", count, name, lastStatus)
		}

		count++

		select {
		case <-ctx.Done():
			return lastStatus

		case <-time.After(1 * time.Second):
		}
	}
}

const (
	storageRetryAttempts = 3
	storageSettleDelay   = 10 * time.Second
	instanceStopTimeout  = 2 * time.Minute
)

// transientStorageErrors are fragments of error messages, which indicate a
// transient problem of the storage backend and are therefore worth a retry.
var transientStorageErrors = []string{
	"zvol",
	"failed unmounting instance",
	"dataset is busy",
	"device or resource busy",
}

func isTransientStorageError(resp cmdResponse) bool {
	output := strings.ToLower(resp.Output())

	for _, fragment := range transientStorageErrors {
		if strings.Contains(output, fragment) {
			return true
		}
	}

	return false
}

// sleepWithContext sleeps for the given duration or until the context is done.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-time.After(d):
		return nil
	}
}

// stopInstanceAttempt performs a single attempt to shut the given instance down
// cleanly.
func stopInstanceAttempt(ctx context.Context, t *testing.T, name string) cmdResponse {
	t.Helper()

	timeout := strechedTimeout(instanceStopTimeout)

	ctx, cancel := context.WithTimeout(ctx, timeout+strechedTimeout(30*time.Second))
	defer cancel()

	return runWithContext(ctx, t, `incus stop --timeout %d %s`, int(timeout.Seconds()), name)
}

// stopInstanceWithContext stops the given instance and verifies, that the
// instance is actually stopped afterwards.
func stopInstanceWithContext(ctx context.Context, t *testing.T, name string) error {
	t.Helper()

	var lastErr error

	for attempt := range storageRetryAttempts {
		resp := stopInstanceAttempt(ctx, t, name)
		if resp.Success() {
			return nil
		}

		lastErr = fmt.Errorf("Failed to stop instance %q: %w", name, fmtRunErr(resp))

		status, err := instanceStatusWithContext(ctx, t, name)
		if err != nil {
			return errors.Join(lastErr, err)
		}

		if status != instanceStatusRunning {
			t.Logf("Instance %q is in status %q, tolerating the error of stop attempt %d: %v", name, status, attempt+1, lastErr)

			// Give the storage backend the chance to release the volume of the
			// instance before it is activated again.
			return sleepWithContext(ctx, storageSettleDelay)
		}

		if !isTransientStorageError(resp) {
			return lastErr
		}

		t.Logf("Stop attempt %d for instance %q failed with a transient storage error: %v", attempt+1, name, lastErr)

		if attempt == storageRetryAttempts-1 {
			break
		}

		err = sleepWithContext(ctx, storageSettleDelay)
		if err != nil {
			return errors.Join(lastErr, err)
		}
	}

	return fmt.Errorf("Giving up after %d attempts: %w", storageRetryAttempts, lastErr)
}

func startInstanceWithContext(ctx context.Context, t *testing.T, name string) error {
	t.Helper()

	return retryStorageCmdWithContext(ctx, t, fmt.Sprintf("start instance %q", name), `incus start %s`, name)
}

func removeInstanceWithContext(ctx context.Context, t *testing.T, name string) error {
	t.Helper()

	return retryStorageCmdWithContext(ctx, t, fmt.Sprintf("remove instance %q", name), `incus remove --force %s`, name)
}

func restartInstanceWithContext(ctx context.Context, t *testing.T, name string) error {
	t.Helper()

	err := stopInstanceWithContext(ctx, t, name)
	if err != nil {
		return err
	}

	return startInstanceWithContext(ctx, t, name)
}

// retryStorageCmdWithContext runs the given command and retries it, as long as
// it fails with a transient error of the storage backend.
func retryStorageCmdWithContext(ctx context.Context, t *testing.T, desc string, command string, args ...any) error {
	t.Helper()

	var lastErr error

	for attempt := range storageRetryAttempts {
		resp := runWithContext(ctx, t, command, args...)
		if resp.Success() {
			return nil
		}

		lastErr = fmt.Errorf("Failed to %s: %w", desc, fmtRunErr(resp))

		if !isTransientStorageError(resp) {
			return lastErr
		}

		t.Logf("Attempt %d to %s failed with a transient storage error: %v", attempt+1, desc, lastErr)

		if attempt == storageRetryAttempts-1 {
			break
		}

		err := sleepWithContext(ctx, storageSettleDelay)
		if err != nil {
			return errors.Join(lastErr, err)
		}
	}

	return fmt.Errorf("Giving up after %d attempts: %w", storageRetryAttempts, lastErr)
}

func mustGetInstanceIPAndNames(t *testing.T, names []string) (instanceIPs []string, instanceNames []string) {
	t.Helper()

	instanceIPs = make([]string, 0, len(names))
	instanceNames = make([]string, 0, len(names))
	for _, name := range names {
		// Get the first IP address not from incusbr0 or meshbr0 with global scope
		// while preferring IPv6.
		ipResp := mustRun(t, `incus list -f json | jq -r '[ .[] | select(.name == "%s") | .state.network | to_entries[] | select(.key != "incusbr0" and .key != "meshbr0") | .value.addresses[]? | select(.scope == "global") | . ] | sort_by(.family) | reverse | first | .address'`, name)
		instanceIPs = append(instanceIPs, strings.TrimSpace(ipResp.Output()))

		nameResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "%s") | .state.os_info.hostname'`, name)
		instanceNames = append(instanceNames, strings.TrimSpace(nameResp.Output()))
	}

	return instanceIPs, instanceNames
}

func setServerBMCConfigWithContext(ctx context.Context, t *testing.T, tmpDir string, name string, apiType string, endpoint string) error {
	t.Helper()

	serverPutFilename := filepath.Join(tmpDir, fmt.Sprintf("server_put_%s.json", name))

	resp := runWithContext(ctx, t, `../bin/operations-center.linux.%[1]s provisioning server show %[2]s -f json | jq -ce --arg api_type '%[3]s' --arg endpoint '%[4]s' '{ public_connection_url, channel, description, properties, bmc_config: { api_type: $api_type, endpoint: $endpoint, certificate: "", auto_pin_certificate: ($api_type != ""), username: "", password: "" } }' > %[5]s`, cpuArch, name, apiType, endpoint, serverPutFilename)

	err := fmtRunErr(resp)
	if err != nil {
		return fmt.Errorf("Failed to assemble the server config for %q: %w", name, err)
	}

	resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server edit %s < %s`, cpuArch, name, serverPutFilename)

	return fmtRunErr(resp)
}

func mustSetServerBMCConfig(ctx context.Context, t *testing.T, tmpDir string, name string, apiType string, endpoint string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	err := setServerBMCConfigWithContext(ctx, t, tmpDir, name, apiType, endpoint)
	require.NoErrorf(t, err, "Failed to set the BMC config of server %q", name)
}

func serverBMCConfigCleanup(t *testing.T, tmpDir string, name string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "server BMC config cleanup")
		defer stop()

		err := setServerBMCConfigWithContext(ctx, t, tmpDir, name, "", "")
		if err != nil {
			t.Logf("Failed to reset the BMC config of server %q: %v", name, err)
		}
	}
}

func setSystemSecurityOIDCWithContext(ctx context.Context, t *testing.T, tmpDir string, issuer string, clientID string, audience string, claim string) error {
	t.Helper()

	securityPutFilename := filepath.Join(tmpDir, "system_security_put.json")

	resp := runWithContext(ctx, t, `../bin/operations-center.linux.%[1]s system security show -f json | jq -ce --arg issuer '%[2]s' --arg client_id '%[3]s' --arg audience '%[4]s' --arg claim '%[5]s' '.oidc = { issuer: $issuer, client_id: $client_id, scopes: "", audience: $audience, claim: $claim }' > %[6]s`, cpuArch, issuer, clientID, audience, claim, securityPutFilename)

	err := fmtRunErr(resp)
	if err != nil {
		return fmt.Errorf("Failed to assemble the security config: %w", err)
	}

	// `length > 0 guards against applying a truncated security config, which
	// would drop the trusted TLS client certificate fingerprints and therefore
	// lock the remaining tests out of Operations Center.
	resp = runWithContext(ctx, t, `jq -e '.trusted_tls_client_cert_fingerprints | length > 0' %s > /dev/null`, securityPutFilename)

	err = fmtRunErr(resp)
	if err != nil {
		return fmt.Errorf("Refusing to apply a security config without trusted TLS client certificate fingerprints: %w", err)
	}

	resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s system security edit < %s`, cpuArch, securityPutFilename)

	return fmtRunErr(resp)
}

func mustSetSystemSecurityOIDC(ctx context.Context, t *testing.T, tmpDir string, issuer string, clientID string, audience string, claim string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	err := setSystemSecurityOIDCWithContext(ctx, t, tmpDir, issuer, clientID, audience, claim)
	require.NoErrorf(t, err, "Failed to set the OIDC security config with issuer %q", issuer)
}

func systemSecurityOIDCCleanup(t *testing.T, tmpDir string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(60*time.Second))
		defer cancel()

		stop := timeTrack(t, "system security OIDC config cleanup")
		defer stop()

		err := setSystemSecurityOIDCWithContext(ctx, t, tmpDir, "", "", "", "")
		if err != nil {
			t.Logf("Failed to reset the OIDC security config: %v", err)
		}
	}
}

func serverPowerStateCleanup(t *testing.T, name string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached
		// context. The budget accommodates the retries of
		// startInstanceWithContext.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(5*time.Minute))
		defer cancel()

		resp := runWithContext(ctx, t, `incus list -f json | jq -r -e '[ .[] | select(.name == "%s" and .status == "Running") ] | length == 1'`, name)
		if resp.Success() {
			return
		}

		stop := timeTrack(t, "server power state cleanup")
		defer stop()

		err := startInstanceWithContext(ctx, t, name)
		if err != nil {
			t.Error(err)
		}
	}
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

var (
	// debugOutputMu guards debugOutput, which is written concurrently by the
	// errgroup goroutines, which set up the VMs.
	debugOutputMu sync.Mutex
	debugOutput   = &bytes.Buffer{}
)

// resetDebugOutput discards the debug output collected so far. It has to be
// called at the start of every test, since the test binary runs all the test
// cases in the same process.
func resetDebugOutput() {
	debugOutputMu.Lock()
	defer debugOutputMu.Unlock()

	debugOutput = &bytes.Buffer{}
}

// takeDebugOutput returns the debug output collected so far.
func takeDebugOutput() []byte {
	debugOutputMu.Lock()
	defer debugOutputMu.Unlock()

	return debugOutput.Bytes()
}

// debugf prints debug messages to stdout, if the global debug variable is true.
// This can be configured by the
// OPERATIONS_CENTER_E2E_TEST_DEBUG env var.
//
// Note, that with the debug output enabled, everything goes to stdout instead
// of the buffer, which leaves the debug_output_*.log written by
// onTestFailDebugOutput empty.
func debugf(format string, args ...any) {
	debugOutputMu.Lock()
	defer debugOutputMu.Unlock()

	var out io.Writer = debugOutput

	if debug {
		out = os.Stdout
	}

	// We don't care about errors here.
	_, _ = fmt.Fprintln(out, indent(fmt.Sprintf(format, args...), "debug: "))
}

const (
	debugJournalSinceGrace = 1 * time.Minute
	debugJournalMaxLines   = 50000
)

func onTestFailDebugOutput(t *testing.T, tmpDir string) func() {
	t.Helper()

	start := time.Now()

	return func() {
		// Print additional debug information in the case of an error.
		if !t.Failed() {
			return
		}

		// Deliberately derived from t.Context() and not from the context of the
		// test, since the debug output has to be collected even (and
		// especially) if the test failed, because it exceeded its timeout.
		ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(30*time.Second))
		defer cancel()

		timestamp := time.Now().Format("2006-01-02-15-04-05")

		fmt.Println("===[ DEBUG OUTPUT ]===")
		debugOutputFilename := filepath.Join(tmpDir, fmt.Sprintf("debug_output_%s.log", timestamp))
		fmt.Printf("Debug output saved in %q\n", debugOutputFilename)
		collectedDebugOutput := takeDebugOutput()

		err := os.WriteFile(debugOutputFilename, collectedDebugOutput, 0o600)
		if err != nil {
			t.Errorf("Failed to write debug output to %q: %v", debugOutputFilename, err)

			// Writing fails, if the filesystem is full, so fall back to
			// stdout.
			fmt.Println(string(collectedDebugOutput))
		}

		operationsCenterJournalFilename := filepath.Join(tmpDir, fmt.Sprintf("operations-center_journal_%s.log", timestamp))
		fmt.Printf("operations-center journal saved in %q\n", operationsCenterJournalFilename)

		journalSince := int((time.Since(start) + debugJournalSinceGrace).Seconds())
		resp := runWithContext(ctx, t, `incus exec OperationsCenter -- journalctl -u operations-center --no-pager --since "-%ds" -n %d`, journalSince, debugJournalMaxLines)
		if !resp.Success() {
			t.Logf("Failed to get the operations-center journal: %s", resp.Error())
		} else {
			err = os.WriteFile(operationsCenterJournalFilename, resp.output.Bytes(), 0o600)
			if err != nil {
				t.Errorf("Failed to write operations-center journal to %q: %v", operationsCenterJournalFilename, err)
			}
		}

		resp = runWithContext(ctx, t, `incus list -f json | jq -r '.[] | select(.name | test("Incus.*|OperationsCenter")) | .name'`)
		if !resp.Success() {
			t.Logf("Failed to list the instances: %s", resp.Error())
		} else {
			for instance := range strings.Lines(resp.OutputTrimmed()) {
				instance = strings.TrimSpace(instance)

				consoleFilename := filepath.Join(tmpDir, fmt.Sprintf("incus_%s_console_%s.log", instance, timestamp))
				fmt.Printf("incus %q console log saved in %q\n", instance, consoleFilename)

				consoleResp := runQuietWithContext(ctx, t, `incus console %s --show-log`, instance)
				if !consoleResp.Success() {
					t.Logf("Failed to get the console log of %q: %s", instance, consoleResp.Error())
				} else {
					err = os.WriteFile(consoleFilename, []byte(ansiEscapeSequence.ReplaceAllString(consoleResp.Output(), "")), 0o600)
					if err != nil {
						t.Errorf("Failed to write incus %q console log to %q: %v", instance, consoleFilename, err)
					}
				}

				if !strings.HasPrefix(instance, "IncusOS") {
					continue
				}

				incusJournalFilename := filepath.Join(tmpDir, fmt.Sprintf("incus_%s_journal_%s.log", instance, timestamp))
				fmt.Printf("incus %q journal saved in %q\n", instance, incusJournalFilename)

				resp := runWithContext(ctx, t, `incus exec %s -- journalctl -u incus -n 1000`, instance)
				if !resp.Success() {
					t.Logf("Failed to get the incus journal of %q: %s", instance, resp.Error())
				} else {
					err = os.WriteFile(incusJournalFilename, resp.output.Bytes(), 0o600)
					if err != nil {
						t.Errorf("Failed to write incus %q journal to %q: %v", instance, incusJournalFilename, err)
					}
				}
			}
		}
	}
}
