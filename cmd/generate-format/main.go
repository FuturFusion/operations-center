// Command generate-format applies gofmt and goimports to the files produced by a
// code generator.
//
// Directives for go:generate are not run through a shell, so globs in a directive
// are not expanded. generate-format takes the patterns verbatim and expands them
// itself, relative to the directory the generator ran in. This keeps the
// formatting pass scoped to the generated files instead of recursing over the
// whole package tree, which is what makes goimports expensive.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

const localPrefix = "github.com/FuturFusion/operations-center"

func main() {
	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-format: %v\n", err)
		os.Exit(1)
	}
}

func run(patterns []string) error {
	if len(patterns) == 0 {
		return fmt.Errorf("Usage: generate-format <pattern>...")
	}

	files := []string{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("Invalid pattern %q: %w", pattern, err)
		}

		for _, match := range matches {
			if !slices.Contains(files, match) {
				files = append(files, match)
			}
		}
	}

	if len(files) == 0 {
		// Nothing was generated, nothing to format.
		return nil
	}

	slices.Sort(files)

	err := command("gofmt", append([]string{"-s", "-w"}, files...))
	if err != nil {
		return err
	}

	return command("go", append([]string{"run", "golang.org/x/tools/cmd/goimports", "-w", "-local", localPrefix}, files...))
}

func command(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}

	return nil
}
