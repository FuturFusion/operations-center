// Command generate-components generates the list of log components from the
// logger.RegisterComponent calls and the list of metrics components from the
// metrics.RegisterComponent calls in the code base. The generated files are not
// standalone documents. doc/reference/settings.md includes them.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	modulePath = "github.com/FuturFusion/operations-center"

	registerFuncName = "RegisterComponent"
)

// target is a package with a RegisterComponent function and the file generated
// from the calls of it.
type target struct {
	kind       string
	pkgPath    string
	outputPath string
	intro      string
}

var targets = []target{
	{
		kind:       "log",
		pkgPath:    modulePath + "/internal/util/logger",
		outputPath: "doc/reference/settings_log_components.md",
		intro: `This is the complete list of the log components the daemon registers, grouped
by their top level. A level configured for a component applies to all of its
children as well, so a top level name is enough to cover everything below it.
`,
	},
	{
		kind:       "metrics",
		pkgPath:    modulePath + "/internal/util/metrics",
		outputPath: "doc/reference/settings_metrics_components.md",
		intro: `This is the complete list of the metrics components the daemon registers,
grouped by their top level. Enabling a component enables all of its children as
well, so a top level name is enough to cover everything below it.
`,
	},
}

// skippedDirs are not part of the daemon, so components registered there are
// not configurable.
var skippedDirs = []string{".git", "doc", "e2e_tests", "ui"}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-components: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	components, err := collectComponents(".")
	if err != nil {
		return err
	}

	// Check all targets first. An error must not leave partial output.
	for _, t := range targets {
		if len(components[t.pkgPath]) == 0 {
			return fmt.Errorf("No %s call found, the code base can not be without %s components", registerFuncName, t.kind)
		}
	}

	for _, t := range targets {
		err = os.WriteFile(t.outputPath, render(t.intro, components[t.pkgPath]), 0o644)
		if err != nil {
			return fmt.Errorf("Failed to write %q: %w", t.outputPath, err)
		}
	}

	return nil
}

// collectComponents returns the names passed to RegisterComponent in the go
// files below root, in lexical order and without duplicates. The names are
// keyed by the import path of the package the called function belongs to.
func collectComponents(root string) (map[string][]string, error) {
	seen := map[string]map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != root && slices.Contains(skippedDirs, d.Name()) {
				return fs.SkipDir
			}

			return nil
		}

		// Test files register components of their own, which are not part of
		// the daemon.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		names, err := componentsInFile(root, path)
		if err != nil {
			return err
		}

		for pkgPath, pkgNames := range names {
			if seen[pkgPath] == nil {
				seen[pkgPath] = map[string]struct{}{}
			}

			for _, name := range pkgNames {
				seen[pkgPath][name] = struct{}{}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	components := make(map[string][]string, len(seen))
	for pkgPath, names := range seen {
		components[pkgPath] = slices.Sorted(maps.Keys(names))
	}

	return components, nil
}

func componentsInFile(root string, path string) (map[string][]string, error) {
	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse %q: %w", path, err)
	}

	relDir, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("Failed to get the package path of %q: %w", path, err)
	}

	filePkgPath := modulePath + "/" + filepath.ToSlash(relDir)

	names := map[string][]string{}

	var skipped []string

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}

		pkgPath, ok := registerComponentPkgPath(file, filePkgPath, call.Fun)
		if !ok {
			return true
		}

		// A name which is not a plain string literal can not be resolved here,
		// and silently leaving the component out would make the reference
		// incomplete without anybody noticing.
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			skipped = append(skipped, fileSet.Position(call.Pos()).String())

			return true
		}

		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			skipped = append(skipped, fileSet.Position(call.Pos()).String())

			return true
		}

		names[pkgPath] = append(names[pkgPath], name)

		return true
	})

	if len(skipped) > 0 {
		return nil, fmt.Errorf("%s is called with something other than a string literal at %s, extend this generator to resolve it", registerFuncName, strings.Join(skipped, ", "))
	}

	return names, nil
}

// registerComponentPkgPath returns the import path of the package that owns
// the RegisterComponent function called by expr. The call is either qualified,
// for example logger.RegisterComponent, or unqualified inside the package
// itself. Calls of packages that are not a target are not reported.
func registerComponentPkgPath(file *ast.File, filePkgPath string, expr ast.Expr) (string, bool) {
	pkgPath := ""

	switch fun := expr.(type) {
	case *ast.SelectorExpr:
		qualifier, ok := fun.X.(*ast.Ident)
		if !ok || fun.Sel.Name != registerFuncName {
			return "", false
		}

		pkgPath = importPath(file, qualifier.Name)

	case *ast.Ident:
		if fun.Name != registerFuncName {
			return "", false
		}

		pkgPath = filePkgPath
	}

	isTarget := slices.ContainsFunc(targets, func(t target) bool {
		return t.pkgPath == pkgPath
	})

	return pkgPath, isTarget
}

// importPath returns the import path the given qualifier refers to in file.
func importPath(file *ast.File, qualifier string) string {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}

		name := path[strings.LastIndexByte(path, '/')+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}

		if name == qualifier {
			return path
		}
	}

	return ""
}

func render(intro string, components []string) []byte {
	buf := &bytes.Buffer{}

	fmt.Fprintf(buf, "<!-- Code generated by generate-components; DO NOT EDIT. -->\n\n%s", intro)

	group := ""

	for _, component := range components {
		topLevel, _, _ := strings.Cut(component, ".")
		if topLevel != group {
			group = topLevel

			fmt.Fprintf(buf, "\n%s:\n\n", topLevel)
		}

		fmt.Fprintf(buf, "- `%s`\n", component)
	}

	return buf.Bytes()
}
