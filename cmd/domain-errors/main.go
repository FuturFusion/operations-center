// Command domain-errors keeps the quality of the errors, which are reported to
// the user, from degrading.
//
// It reports the places, which do not follow the conventions of
// doc/development/error-handling.md, and fails, if it finds any.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FuturFusion/operations-center/cmd/domain-errors/scan"
)

func main() {
	reportOnly := flag.Bool("report-only", false, "Report the findings without failing")

	flag.Usage = func() {
		out := flag.CommandLine.Output()

		_, _ = fmt.Fprintf(out, `Usage: domain-errors [flags] [packages]

Reports the places, which do not follow the error handling conventions
of doc/development/error-handling.md.

Without a package, ./... is inspected.

`)
		flag.PrintDefaults()
	}

	flag.Parse()

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	err := run(patterns, *reportOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "domain-errors: %v\n", err)
		os.Exit(1)
	}
}

func run(patterns []string, reportOnly bool) error {
	pkgs, err := scan.Load(patterns...)
	if err != nil {
		return err
	}

	result, err := scan.Scan(pkgs)
	if err != nil {
		return err
	}

	findings := check(result)

	for _, f := range findings {
		_, err = fmt.Fprintf(os.Stdout, "%s: %s: %s\n", f.Pos, f.Rule, f.Msg)
		if err != nil {
			return fmt.Errorf("Failed to report the findings: %w", err)
		}
	}

	if len(findings) == 0 || reportOnly {
		return nil
	}

	return fmt.Errorf("%d finding(s), see doc/development/error-handling.md for the conventions", len(findings))
}
