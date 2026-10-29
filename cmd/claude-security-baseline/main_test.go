package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

// Run "go test github.com/FuturFusion/operations-center/cmd/claude-security-baseline/ -update-goldenfiles" to update the golden files automatically.
var updateGoldenfiles = flag.Bool("update-goldenfiles", false, "golden files are updated, if this flag is provided")

func fixedNow() time.Time {
	return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string

		assertErr  require.ErrorAssertionFunc
		wantStdout string
	}{
		{
			name: "help",
			args: []string{"help"},

			assertErr:  require.NoError,
			wantStdout: usage,
		},
		{
			name: "missing command",
			args: nil,

			assertErr: errassert.Contains("missing command"),
		},
		{
			name: "unknown command",
			args: []string{"bogus"},

			assertErr: errassert.Contains(`unknown command "bogus"`),
		},
		{
			name: "help of a command",
			args: []string{"filter", "-h"},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, flag.ErrHelp, a...)
			},
		},
		{
			name: "filter without report directory",
			args: []string{"filter"},

			assertErr: errassert.Contains("exactly one report directory"),
		},
		{
			name: "filter with invalid flag",
			args: []string{"filter", "-bogus", "testdata/report"},

			assertErr: errassert.Contains("flag provided but not defined"),
		},
		{
			name: "add without findings",
			args: []string{"add", "-status", "accepted", "-reason", "x", "testdata/report"},

			assertErr: errassert.Contains("at least one finding"),
		},
		{
			name: "add with invalid flag",
			args: []string{"add", "-bogus"},

			assertErr: errassert.Contains("flag provided but not defined"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := run(tc.args, &stdout, &stderr, fixedNow)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantStdout, stdout.String(), "stdout")
		})
	}
}

func TestRunFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseline  string
		reportDir string

		assertErr     require.ErrorAssertionFunc
		wantStdout    string
		wantGoldenDir string
	}{
		{
			name:      "baseline with entries",
			baseline:  "testdata/baseline.yaml",
			reportDir: "testdata/report",

			assertErr: require.NoError,
			wantStdout: `2 of 4 findings suppressed by the baseline.
Remaining findings: 1 HIGH, 1 LOW.
Hint: F1 has the same file and CWE as baseline entry v3:old.
Hint: F4 has the same file and CWE as baseline entry v3:old.
1 baseline entries match no finding in this report. The code may have changed, or the scan did not cover the file:
  v3:old  internal/tls.go  TLS chain
Wrote CLAUDE-SECURITY-RESULTS-FILTERED.{md,jsonl,sarif} to <out>.
`,
			wantGoldenDir: "testdata/golden/filtered",
		},
		{
			name:      "empty baseline",
			baseline:  "testdata/baseline-empty.yaml",
			reportDir: "testdata/report",

			assertErr: require.NoError,
			wantStdout: `0 of 4 findings suppressed by the baseline.
Remaining findings: 1 HIGH, 2 MEDIUM, 1 LOW.
Wrote CLAUDE-SECURITY-RESULTS-FILTERED.{md,jsonl,sarif} to <out>.
`,
			wantGoldenDir: "testdata/golden/unfiltered",
		},
		{
			name:      "missing baseline",
			baseline:  "testdata/missing.yaml",
			reportDir: "testdata/report",

			assertErr:     errassert.Contains("read baseline"),
			wantGoldenDir: "testdata/golden/none",
		},
		{
			name:      "invalid baseline",
			baseline:  "testdata/report/CLAUDE-SECURITY-RESULTS.md",
			reportDir: "testdata/report",

			assertErr:     errassert.Contains("parse baseline"),
			wantGoldenDir: "testdata/golden/none",
		},
		{
			name:      "missing report",
			baseline:  "testdata/baseline.yaml",
			reportDir: "testdata/missing",

			assertErr:     errassert.Contains("read report"),
			wantGoldenDir: "testdata/golden/none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			outDir := t.TempDir()
			var stdout, stderr bytes.Buffer

			err := run([]string{"filter", "-baseline", tc.baseline, "-out-dir", outDir, tc.reportDir}, &stdout, &stderr, fixedNow)

			tc.assertErr(t, err)
			require.Equal(t, strings.ReplaceAll(tc.wantStdout, "<out>", outDir), stdout.String(), "stdout")
			require.Empty(t, stderr.String(), "stderr")
			requireGoldenDir(t, tc.wantGoldenDir, outDir)
		})
	}
}

func TestRunAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseline string
		args     []string

		assertErr        require.ErrorAssertionFunc
		wantStdout       string
		wantBaselineFile string
	}{
		{
			name:     "add to empty baseline",
			baseline: "testdata/baseline-empty.yaml",
			args:     []string{"-status", "accepted", "-reason", "Only reachable by admins.", "-by", "tester", "testdata/report", "F1", "v3:ddd"},

			assertErr: require.NoError,
			wantStdout: `Add F1 (v3:aaa).
Add F4 (v3:ddd).
`,
			wantBaselineFile: "testdata/golden/add-empty.yaml",
		},
		{
			name:     "add to existing baseline, skip known ids",
			baseline: "testdata/baseline.yaml",
			args:     []string{"-status", "false-positive", "-reason", "Not reachable.", "testdata/report", "F3", "F1", "F1"},

			assertErr: require.NoError,
			wantStdout: `Skip F3: v3:bbb is already in the baseline.
Add F1 (v3:aaa).
Skip F1: v3:aaa is already in the baseline.
`,
			wantBaselineFile: "testdata/golden/add-existing.yaml",
		},
		{
			name:     "nothing new",
			baseline: "testdata/baseline.yaml",
			args:     []string{"-status", "accepted", "-reason", "x", "testdata/report", "F2"},

			assertErr:        require.NoError,
			wantStdout:       "Skip F2: v3:bbb is already in the baseline.\n",
			wantBaselineFile: "testdata/baseline.yaml",
		},
		{
			name:     "unknown finding",
			baseline: "testdata/baseline.yaml",
			args:     []string{"-status", "accepted", "-reason", "x", "testdata/report", "F1", "F9"},

			assertErr:        errassert.Contains("finding F9 is not in the report"),
			wantStdout:       "Add F1 (v3:aaa).\n",
			wantBaselineFile: "testdata/baseline.yaml",
		},
		{
			name:     "invalid status",
			baseline: "testdata/baseline.yaml",
			args:     []string{"-status", "bogus", "-reason", "x", "testdata/report", "F1"},

			assertErr:        errassert.Contains("status must be one of accepted, false-positive"),
			wantBaselineFile: "testdata/baseline.yaml",
		},
		{
			name:     "missing reason",
			baseline: "testdata/baseline.yaml",
			args:     []string{"-status", "accepted", "testdata/report", "F1"},

			assertErr:        errassert.Contains("missing reason"),
			wantBaselineFile: "testdata/baseline.yaml",
		},
		{
			name:     "missing report",
			baseline: "testdata/baseline.yaml",
			args:     []string{"-status", "accepted", "-reason", "x", "testdata/missing", "F1"},

			assertErr:        errassert.Contains("read report"),
			wantBaselineFile: "testdata/baseline.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			baselinePath := filepath.Join(t.TempDir(), "baseline.yaml")
			data, err := os.ReadFile(tc.baseline)
			require.NoError(t, err)
			err = os.WriteFile(baselinePath, data, 0o644)
			require.NoError(t, err)

			var stdout, stderr bytes.Buffer

			// Run test
			err = run(append([]string{"add", "-baseline", baselinePath}, tc.args...), &stdout, &stderr, fixedNow)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantStdout, stdout.String(), "stdout")
			requireGoldenFile(t, tc.wantBaselineFile, baselinePath)
		})
	}
}

func TestRunAdd_missingBaseline(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	err := run([]string{"add", "-baseline", "testdata/missing.yaml", "-status", "accepted", "-reason", "x", "testdata/report", "F1"}, &stdout, &stderr, fixedNow)

	require.ErrorContains(t, err, "read baseline")
}

// requireGoldenDir compares all files in gotDir with the files in goldenDir.
// A missing golden directory stands for an empty directory.
func requireGoldenDir(t *testing.T, goldenDir string, gotDir string) {
	t.Helper()

	got := readDir(t, gotDir)
	if *updateGoldenfiles && len(got) > 0 {
		err := os.MkdirAll(goldenDir, 0o755)
		require.NoError(t, err)

		for name, data := range got {
			err = os.WriteFile(filepath.Join(goldenDir, name), []byte(data), 0o644)
			require.NoError(t, err)
		}
	}

	require.Equal(t, readDir(t, goldenDir), got, "files in %s compared with %s", gotDir, goldenDir)
}

func requireGoldenFile(t *testing.T, goldenFile string, gotFile string) {
	t.Helper()

	got, err := os.ReadFile(gotFile)
	require.NoError(t, err)

	if *updateGoldenfiles && strings.HasPrefix(goldenFile, "testdata/golden/") {
		err = os.WriteFile(goldenFile, got, 0o644)
		require.NoError(t, err)
	}

	want, err := os.ReadFile(goldenFile)
	require.NoError(t, err)

	require.Equal(t, string(want), string(got), "file %s compared with %s", gotFile, goldenFile)
}

func readDir(t *testing.T, dir string) map[string]string {
	t.Helper()

	files := map[string]string{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return files
	}

	require.NoError(t, err)

	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)

		files[e.Name()] = string(data)
	}

	return files
}
