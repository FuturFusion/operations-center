// Command claude-security-baseline filters Claude Security scan reports
// against a baseline of triaged findings.
//
// The baseline lists findings that the team reviewed and marked as "accepted"
// or "false-positive". A finding matches a baseline entry by its
// claudeSecurityPluginFindingId. The plugin computes this ID from the rule, the
// file path and the code around the finding. The ID changes when this code
// changes. The finding then shows up again and needs a new review.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	defaultBaseline = ".claude-security-baseline.yaml"

	usage = `Usage:
  claude-security-baseline filter [flags] <report dir>
  claude-security-baseline add [flags] <report dir> <finding>...

Commands:
  filter  Write filtered copies of a report next to the original files.
  add     Add findings of a report to the baseline. A finding is a finding
          number (F3) or a finding ID (v3:...).

Run "claude-security-baseline <command> -h" for the flags of a command.
`
)

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now)
	if errors.Is(err, flag.ErrHelp) {
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-security-baseline: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer, now func() time.Time) error {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return errors.New("missing command")
	}

	switch args[0] {
	case "filter":
		return runFilter(args[1:], stdout, stderr)

	case "add":
		return runAdd(args[1:], stdout, stderr, now)

	case "-h", "-help", "--help", "help":
		_, _ = fmt.Fprint(stdout, usage)
		return nil

	default:
		_, _ = fmt.Fprint(stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runFilter(args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("filter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	baselinePath := flags.String("baseline", defaultBaseline, "baseline file")
	outDir := flags.String("out-dir", "", "output directory (default: the report directory)")

	err := flags.Parse(args)
	if err != nil {
		return err
	}

	if flags.NArg() != 1 {
		return errors.New("filter needs exactly one report directory")
	}

	reportDir := flags.Arg(0)
	if *outDir == "" {
		*outDir = reportDir
	}

	return filterReport(reportDir, *baselinePath, *outDir, stdout, stderr)
}

func runAdd(args []string, stdout io.Writer, stderr io.Writer, now func() time.Time) error {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(stderr)
	baselinePath := flags.String("baseline", defaultBaseline, "baseline file")
	status := flags.String("status", "", `"accepted" or "false-positive" (required)`)
	reason := flags.String("reason", "", "why the finding is accepted or not a real issue (required)")
	reviewer := flags.String("by", "", "name of the reviewer")

	err := flags.Parse(args)
	if err != nil {
		return err
	}

	if flags.NArg() < 2 {
		return errors.New("add needs a report directory and at least one finding")
	}

	return addFindings(addOptions{
		reportDir:    flags.Arg(0),
		baselinePath: *baselinePath,
		wanted:       flags.Args()[1:],
		status:       *status,
		reason:       *reason,
		reviewer:     *reviewer,
		date:         now().Format(time.DateOnly),
	}, stdout)
}
