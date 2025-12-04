package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"

	"github.com/FuturFusion/operations-center/cmd/generate-expr/expr"
	"github.com/FuturFusion/operations-center/cmd/generate-expr/lex"
)

func generateCmd() *cobra.Command {
	var pkgs *[]string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate code for all `//generate-expr: <struct>` declarations.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return generate(*pkgs)
		},
	}

	flags := cmd.Flags()
	pkgs = flags.StringArrayP("package", "p", []string{}, "Packages allowed to inspect")

	return cmd
}

const prefix = "//generate-expr: "

func generate(pkgPaths []string) error {
	localPath, err := os.Getwd()
	if err != nil {
		return err
	}

	// Delete existing _expr_gen.go files before proceeding so they don't taint the parsed packages.
	glob, err := filepath.Glob("*" + expr.FilePrefix)
	if err != nil {
		return fmt.Errorf("Failed to find _expr_gen.go files: %w", err)
	}

	for _, f := range glob {
		err := os.Remove(f)
		if err != nil {
			return fmt.Errorf("Failed to remove %q: %w", f, err)
		}
	}

	localPkg, err := packages.Load(&packages.Config{Mode: packages.NeedName}, localPath)
	if err != nil {
		return err
	}

	localPkgPath := localPkg[0].PkgPath
	pkgs := []string{}
	aliases := map[string]string{}
	if len(pkgPaths) == 0 {
		pkgs = []string{localPkgPath}
		aliases[localPkgPath] = ""
	}

	// Add a utilities file.
	utilFile, err := os.Create(expr.HelperFile)
	if err != nil {
		return err
	}

	defer utilFile.Close()

	_, err = fmt.Fprintf(utilFile, expr.UtilHelpers, filepath.Base(localPkgPath))
	if err != nil {
		return err
	}

	testFile, err := os.Create(expr.TestHelperFile)
	if err != nil {
		return err
	}

	defer testFile.Close()

	_, err = fmt.Fprintf(testFile, expr.TestHelpers, filepath.Base(localPkgPath))
	if err != nil {
		return err
	}

	for _, p := range pkgPaths {
		alias, path, ok := strings.Cut(p, ":")
		if !ok {
			path = p
			alias = filepath.Base(p)
		}

		if path == localPkgPath {
			alias = ""
		}

		existing, ok := aliases[path]
		if ok {
			return fmt.Errorf("Package %q imported as %q clashes with existing package %q (%q)", p, alias, path, existing)
		}

		aliases[path] = alias
		pkgs = append(pkgs, path)
	}

	parsedPkgs, err := packageLoad(pkgs)
	if err != nil {
		return err
	}

	p, err := expr.NewParser(localPkgPath, parsedPkgs, aliases)
	if err != nil {
		return fmt.Errorf("Failed to create parser: %w", err)
	}

	for _, parsedPkg := range parsedPkgs {
		for _, goFile := range parsedPkg.CompiledGoFiles {
			body, err := os.ReadFile(goFile)
			if err != nil {
				return fmt.Errorf("Failed to read %v: %w", goFile, err)
			}

			for line := range strings.SplitSeq(string(body), "\n") {
				// Lazy matching for prefix, does not consider Go syntax and therefore
				// lines starting with prefix, that are part of e.g. multiline strings
				// match as well. This is highly unlikely to cause false positives.
				after, ok := strings.CutPrefix(line, prefix)
				if ok {
					line = after

					// Use csv parser to properly handle arguments surrounded by double quotes.
					r := csv.NewReader(strings.NewReader(line))
					r.Comma = ' ' // space
					args, err := r.Read()
					if err != nil {
						return fmt.Errorf("Failed to read args: %w", err)
					}

					if len(args) == 0 {
						return errors.New("struct name missing")
					}

					err = p.CopyStruct(args[0], lex.SnakeCase(args[0]))
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func packageLoad(pkgs []string) ([]*packages.Package, error) {
	pkgPaths := []string{}
	for _, pkg := range pkgs {
		if pkg == "" {
			var err error
			localPath, err := os.Getwd()
			if err != nil {
				return nil, err
			}

			pkgPaths = append(pkgPaths, localPath)
		} else {
			importPkg, err := build.Import(pkg, "", build.FindOnly)
			if err != nil {
				return nil, fmt.Errorf("Invalid import path %q: %w", pkg, err)
			}

			pkgPaths = append(pkgPaths, importPkg.Dir)
		}
	}

	parsedPkgs, err := packages.Load(&packages.Config{
		Mode: packages.LoadTypes | packages.NeedTypesInfo,
	}, pkgPaths...)
	if err != nil {
		return nil, err
	}

	return parsedPkgs, nil
}
